#!/usr/bin/env python3
"""Build Homebox's currency list from immutable and validated upstream data."""

import csv
import io
import json
import logging
import os
import re
import sys
import tempfile
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

COUNTRIES_URL = (
    "https://raw.githubusercontent.com/restcountries/restcountries/"
    "bfadee4f951682c29970e53677707bc558e80b74/"
    "src/main/resources/countriesV3.1.json"
)
ISO_4217_URL = (
    "https://raw.githubusercontent.com/datasets/currency-codes/"
    "c553021f125b457909f6511be087c50f08fbd108/data/codes-all.csv"
)
SAVE_PATH = Path("backend/internal/core/currencies/currencies.json")
TIMEOUT_SECONDS = 20
MAX_ATTEMPTS = 3
RETRY_BACKOFF_SECONDS = 1
MAX_DOWNLOAD_BYTES = 5_000_000

# Known currency decimal overrides
CURRENCY_DECIMAL_OVERRIDES = {
    "BTC": 8,  # Bitcoin uses 8 decimal places
    "JPY": 0,  # Japanese Yen has no decimal places
    "BHD": 3,  # Bahraini Dinar uses 3 decimal places
}
DEFAULT_DECIMALS = 2
MIN_DECIMALS = 0
MAX_DECIMALS = 6
MIN_ISO_CURRENCIES = 150
MIN_CURRENCY_ENTRIES = 200
MIN_UNIQUE_CURRENCIES = 150
REQUIRED_ISO_DECIMALS = {"EUR": 2, "JPY": 0, "USD": 2}
CURRENCY_CODE_PATTERN = re.compile(r"^[A-Z]{3}$")


class UpdateError(RuntimeError):
    """Raised when upstream data cannot be trusted or safely written."""


def setup_logging():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s: %(message)s",
    )


def fetch_bytes(url, accept):
    """Fetch a bounded HTTPS resource, retrying transient failures."""
    parsed_url = urlsplit(url)
    if parsed_url.scheme != "https" or not parsed_url.hostname:
        raise UpdateError(f"refusing non-HTTPS URL: {url}")

    request = Request(
        url,
        headers={
            "Accept": accept,
            "User-Agent": "homebox-currency-updater/1.0",
        },
    )

    for attempt in range(1, MAX_ATTEMPTS + 1):
        try:
            with urlopen(request, timeout=TIMEOUT_SECONDS) as response:
                final_url = urlsplit(response.geturl())
                if (
                    final_url.scheme != "https"
                    or final_url.hostname != parsed_url.hostname
                ):
                    raise UpdateError(
                        f"refusing cross-host redirect from {url} to {response.geturl()}"
                    )

                content = response.read(MAX_DOWNLOAD_BYTES + 1)
                if len(content) > MAX_DOWNLOAD_BYTES:
                    raise UpdateError(
                        f"response from {url} exceeds {MAX_DOWNLOAD_BYTES} bytes"
                    )
                return content
        except HTTPError as exc:
            retryable = exc.code == 429 or 500 <= exc.code < 600
            if not retryable or attempt == MAX_ATTEMPTS:
                raise UpdateError(
                    f"request to {url} failed with HTTP {exc.code}"
                ) from exc
            error = f"HTTP {exc.code}"
        except (OSError, TimeoutError, URLError) as exc:
            if attempt == MAX_ATTEMPTS:
                raise UpdateError(f"request to {url} failed: {exc}") from exc
            error = str(exc)

        delay = RETRY_BACKOFF_SECONDS * (2 ** (attempt - 1))
        logging.warning(
            "Request to %s failed (%s); retrying in %d second(s)",
            url,
            error,
            delay,
        )
        time.sleep(delay)

    raise AssertionError("retry loop exited unexpectedly")


def get_currency_decimals(code, iso_data):
    """
    Get the decimal places for a currency code.
    Checks overrides first, then ISO data, then uses default.
    Clamps result to safe range [MIN_DECIMALS, MAX_DECIMALS].
    """
    normalized_code = (code or "").strip().upper()

    if normalized_code in CURRENCY_DECIMAL_OVERRIDES:
        decimals = CURRENCY_DECIMAL_OVERRIDES[normalized_code]
    elif normalized_code in iso_data:
        decimals = iso_data[normalized_code]
    else:
        decimals = DEFAULT_DECIMALS

    try:
        decimals = int(decimals)
    except (ValueError, TypeError):
        decimals = DEFAULT_DECIMALS

    return max(MIN_DECIMALS, min(MAX_DECIMALS, decimals))


def parse_iso_4217_data(content):
    """Parse and validate active ISO 4217 minor-unit data."""
    try:
        csv_content = content.decode("utf-8-sig")
    except UnicodeDecodeError as exc:
        raise UpdateError("ISO 4217 data is not valid UTF-8") from exc

    csv_reader = csv.DictReader(io.StringIO(csv_content))
    required_fields = {"AlphabeticCode", "MinorUnit", "WithdrawalDate"}
    if not csv_reader.fieldnames or not required_fields.issubset(
        csv_reader.fieldnames
    ):
        raise UpdateError("ISO 4217 data is missing required columns")

    iso_data = {}
    for row in csv_reader:
        code = (row.get("AlphabeticCode") or "").strip().upper()
        minor_unit = (row.get("MinorUnit") or "").strip()
        withdrawal_date = (row.get("WithdrawalDate") or "").strip()

        if withdrawal_date or not code or not minor_unit.isdigit():
            continue
        if not CURRENCY_CODE_PATTERN.fullmatch(code):
            raise UpdateError(f"invalid ISO 4217 currency code: {code!r}")

        decimals = int(minor_unit)
        if not MIN_DECIMALS <= decimals <= MAX_DECIMALS:
            raise UpdateError(
                f"invalid ISO 4217 minor unit for {code}: {decimals}"
            )
        if code in iso_data and iso_data[code] != decimals:
            raise UpdateError(f"conflicting ISO 4217 minor units for {code}")
        iso_data[code] = decimals

    if len(iso_data) < MIN_ISO_CURRENCIES:
        raise UpdateError(
            "ISO 4217 data contains only "
            f"{len(iso_data)} active currencies; expected at least "
            f"{MIN_ISO_CURRENCIES}"
        )
    for code, expected_decimals in REQUIRED_ISO_DECIMALS.items():
        if iso_data.get(code) != expected_decimals:
            raise UpdateError(
                f"ISO 4217 data has an unexpected minor unit for {code}"
            )

    return iso_data


def fetch_iso_4217_data():
    """Fetch ISO 4217 currency data and return code-to-minor-unit mappings."""
    logging.info("Fetching ISO 4217 data from immutable source: %s", ISO_4217_URL)
    iso_data = parse_iso_4217_data(fetch_bytes(ISO_4217_URL, "text/csv"))
    logging.info(
        "Loaded decimal data for %d active ISO 4217 currencies", len(iso_data)
    )
    return iso_data


def parse_currencies(content, iso_data):
    """Parse and validate the Rest Countries response."""
    try:
        countries = json.loads(content.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise UpdateError("country dataset is not valid JSON") from exc

    if not isinstance(countries, list):
        raise UpdateError("country dataset must be a list")

    results = []
    for index, country in enumerate(countries):
        if not isinstance(country, dict):
            raise UpdateError(f"country entry {index} must be an object")

        name_field = country.get("name", {})
        if isinstance(name_field, dict):
            country_name = name_field.get("common")
        elif isinstance(name_field, str):
            country_name = name_field
        else:
            raise UpdateError(f"country entry {index} has an invalid name")
        if not isinstance(country_name, str) or not country_name.strip():
            raise UpdateError(f"country entry {index} has no common name")
        country_name = country_name.strip()

        currencies = country.get("currencies", {})
        if currencies is None:
            continue
        if not isinstance(currencies, dict):
            raise UpdateError(
                f"currencies for {country_name} must be an object"
            )

        for raw_code, info in currencies.items():
            if not isinstance(raw_code, str) or not isinstance(info, dict):
                raise UpdateError(f"invalid currency entry for {country_name}")

            code = raw_code.strip().upper()
            if not CURRENCY_CODE_PATTERN.fullmatch(code):
                raise UpdateError(
                    f"invalid currency code {raw_code!r} for {country_name}"
                )

            currency_name = info.get("name", "")
            symbol = info.get("symbol", "")
            if not isinstance(currency_name, str) or not currency_name.strip():
                raise UpdateError(
                    f"currency {code} for {country_name} has no name"
                )
            if not isinstance(symbol, str):
                raise UpdateError(
                    f"currency {code} for {country_name} has an invalid symbol"
                )

            currency_name = currency_name.strip()
            currency_name = currency_name[0].upper() + currency_name[1:]
            decimals = get_currency_decimals(code, iso_data)

            results.append({
                "code": code,
                "local": country_name,
                "symbol": symbol,
                "name": currency_name,
                "decimals": decimals,
            })

    results.sort(key=lambda item: item["local"].casefold())
    validate_currencies(results)
    return results


def validate_currencies(currencies):
    """Reject incomplete or malformed generated datasets."""
    if not isinstance(currencies, list):
        raise UpdateError("generated currency data must be a list")
    if len(currencies) < MIN_CURRENCY_ENTRIES:
        raise UpdateError(
            f"generated only {len(currencies)} currency entries; expected at least "
            f"{MIN_CURRENCY_ENTRIES}"
        )

    codes = set()
    country_codes = set()
    expected_keys = {"code", "local", "symbol", "name", "decimals"}
    for index, currency in enumerate(currencies):
        if not isinstance(currency, dict) or set(currency) != expected_keys:
            raise UpdateError(f"generated currency entry {index} is malformed")

        code = currency["code"]
        local = currency["local"]
        name = currency["name"]
        symbol = currency["symbol"]
        decimals = currency["decimals"]
        if not isinstance(code, str) or not CURRENCY_CODE_PATTERN.fullmatch(code):
            raise UpdateError(f"generated currency entry {index} has an invalid code")
        if not isinstance(local, str) or not local:
            raise UpdateError(f"generated currency entry {index} has no country")
        if not isinstance(name, str) or not name:
            raise UpdateError(f"generated currency entry {index} has no name")
        if not isinstance(symbol, str):
            raise UpdateError(f"generated currency entry {index} has an invalid symbol")
        if (
            type(decimals) is not int
            or not MIN_DECIMALS <= decimals <= MAX_DECIMALS
        ):
            raise UpdateError(
                f"generated currency entry {index} has invalid decimals"
            )

        country_code = (local.casefold(), code)
        if country_code in country_codes:
            raise UpdateError(f"duplicate generated currency {code} for {local}")
        country_codes.add(country_code)
        codes.add(code)

    if len(codes) < MIN_UNIQUE_CURRENCIES:
        raise UpdateError(
            f"generated only {len(codes)} unique currencies; expected at least "
            f"{MIN_UNIQUE_CURRENCIES}"
        )
    missing_codes = REQUIRED_ISO_DECIMALS.keys() - codes
    if missing_codes:
        raise UpdateError(
            "generated currency data is missing required codes: "
            + ", ".join(sorted(missing_codes))
        )


def fetch_currencies():
    """Fetch, merge, and validate all upstream currency data."""
    iso_data = fetch_iso_4217_data()
    logging.info("Fetching countries from immutable source: %s", COUNTRIES_URL)
    return parse_currencies(
        fetch_bytes(COUNTRIES_URL, "application/json"),
        iso_data,
    )


def load_existing(path: Path):
    if not path.exists():
        return None
    try:
        with path.open("r", encoding="utf-8") as file:
            existing = json.load(file)
    except (OSError, json.JSONDecodeError) as exc:
        raise UpdateError(f"could not load existing file {path}: {exc}") from exc
    if not isinstance(existing, list):
        raise UpdateError(f"existing currency file {path} must contain a list")
    return existing


def save_currencies(data, path: Path):
    """Atomically replace the destination with validated JSON."""
    validate_currencies(data)
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary_name = tempfile.mkstemp(
            dir=path.parent,
            prefix=f".{path.name}.",
            suffix=".tmp",
            text=True,
        )
    except OSError as exc:
        raise UpdateError(f"could not create a temporary currency file: {exc}") from exc

    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as file:
            json.dump(data, file, ensure_ascii=False, indent=4)
            file.write("\n")
            file.flush()
            os.fsync(file.fileno())
        os.chmod(temporary_path, 0o644)
        os.replace(temporary_path, path)
    except OSError as exc:
        raise UpdateError(f"could not write currency file {path}: {exc}") from exc
    finally:
        try:
            temporary_path.unlink()
        except FileNotFoundError:
            pass

    logging.info("Wrote %d entries to %s", len(data), path)


def main():
    setup_logging()
    logging.info("Starting currency update")

    try:
        existing = load_existing(SAVE_PATH)
        new = fetch_currencies()
        validate_currencies(new)
        if new == existing:
            logging.info("Currency file is already up to date")
            return 0

        save_currencies(new, SAVE_PATH)
    except UpdateError as exc:
        logging.error("Currency update aborted: %s", exc)
        return 1

    logging.info("Currency file updated")
    return 0


if __name__ == "__main__":
    sys.exit(main())
