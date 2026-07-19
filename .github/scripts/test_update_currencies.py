import csv
import io
import itertools
import json
import string
import tempfile
import unittest
from pathlib import Path
from unittest import mock
from urllib.error import URLError

import update_currencies as updater


def currency_codes(count):
    required = list(updater.REQUIRED_ISO_DECIMALS)
    generated = (
        "".join(characters)
        for characters in itertools.product(string.ascii_uppercase, repeat=3)
    )
    codes = required[:]
    for code in generated:
        if code not in codes:
            codes.append(code)
        if len(codes) == count:
            return codes
    raise AssertionError("not enough currency codes generated")


def valid_iso_csv():
    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(
        [
            "Entity",
            "Currency",
            "AlphabeticCode",
            "NumericCode",
            "MinorUnit",
            "WithdrawalDate",
        ]
    )
    expected_decimals = updater.REQUIRED_ISO_DECIMALS
    for index, code in enumerate(currency_codes(updater.MIN_ISO_CURRENCIES)):
        writer.writerow(
            [
                f"Entity {index}",
                f"Currency {index}",
                code,
                index,
                expected_decimals.get(code, 2),
                "",
            ]
        )
    writer.writerow(["Old Entity", "Old Currency", "OLD", 999, 2, "2020-01"])
    writer.writerow(["Special", "No minor unit", "XXX", 999, "-", ""])
    return output.getvalue().encode()


def valid_iso_data():
    data = {code: 2 for code in currency_codes(updater.MIN_UNIQUE_CURRENCIES)}
    data.update(updater.REQUIRED_ISO_DECIMALS)
    return data


def valid_countries_json():
    codes = currency_codes(updater.MIN_UNIQUE_CURRENCIES)
    countries = []
    for index in range(updater.MIN_CURRENCY_ENTRIES):
        code = codes[index % len(codes)]
        countries.append(
            {
                "name": {"common": f"Country {index:03d}"},
                "currencies": {
                    code: {
                        "name": f"currency {code}",
                        "symbol": f"{index}",
                    }
                },
            }
        )
    return json.dumps(countries).encode()


class FakeResponse:
    def __init__(self, url, content):
        self.url = url
        self.content = content

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return False

    def geturl(self):
        return self.url

    def read(self, size):
        return self.content[:size]


class UpdateCurrenciesTests(unittest.TestCase):
    def test_upstream_sources_are_pinned_to_commits(self):
        self.assertRegex(updater.ISO_4217_URL, r"/[0-9a-f]{40}/")
        self.assertRegex(updater.COUNTRIES_URL, r"/[0-9a-f]{40}/")

    def test_parse_iso_data_accepts_active_rows_only(self):
        data = updater.parse_iso_4217_data(valid_iso_csv())

        self.assertEqual(updater.MIN_ISO_CURRENCIES, len(data))
        self.assertEqual(0, data["JPY"])
        self.assertNotIn("OLD", data)
        self.assertNotIn("XXX", data)

    def test_parse_iso_data_fails_closed_on_incomplete_data(self):
        content = (
            b"AlphabeticCode,MinorUnit,WithdrawalDate\n"
            b"USD,2,\n"
            b"EUR,2,\n"
            b"JPY,0,\n"
        )

        with self.assertRaises(updater.UpdateError):
            updater.parse_iso_4217_data(content)

    def test_parse_currencies_validates_shape_and_scale(self):
        currencies = updater.parse_currencies(
            valid_countries_json(), valid_iso_data()
        )

        self.assertEqual(updater.MIN_CURRENCY_ENTRIES, len(currencies))
        self.assertEqual("Currency EUR", currencies[0]["name"])
        self.assertEqual(2, currencies[0]["decimals"])

    def test_parse_currencies_rejects_non_list_response(self):
        with self.assertRaises(updater.UpdateError):
            updater.parse_currencies(b"{}", valid_iso_data())

    def test_fetch_bytes_retries_transient_errors(self):
        response = FakeResponse(updater.COUNTRIES_URL, b"payload")
        with mock.patch.object(
            updater,
            "urlopen",
            side_effect=[URLError("temporary failure"), response],
        ) as mocked_open, mock.patch.object(updater.time, "sleep") as mocked_sleep:
            content = updater.fetch_bytes(
                updater.COUNTRIES_URL, "application/json"
            )

        self.assertEqual(b"payload", content)
        self.assertEqual(2, mocked_open.call_count)
        mocked_sleep.assert_called_once_with(updater.RETRY_BACKOFF_SECONDS)

    def test_fetch_bytes_rejects_cross_host_redirects(self):
        response = FakeResponse("https://example.com/data", b"payload")
        with mock.patch.object(updater, "urlopen", return_value=response):
            with self.assertRaises(updater.UpdateError):
                updater.fetch_bytes(
                    updater.COUNTRIES_URL, "application/json"
                )

    def test_load_existing_rejects_invalid_json(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "currencies.json"
            path.write_text("{", encoding="utf-8")

            with self.assertRaises(updater.UpdateError):
                updater.load_existing(path)

    def test_save_currencies_replaces_file_atomically(self):
        currencies = updater.parse_currencies(
            valid_countries_json(), valid_iso_data()
        )
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "currencies.json"
            path.write_text("old", encoding="utf-8")

            updater.save_currencies(currencies, path)

            self.assertEqual(currencies, json.loads(path.read_text(encoding="utf-8")))
            self.assertTrue(path.read_bytes().endswith(b"\n"))
            self.assertEqual([], list(path.parent.glob(".*.tmp")))

    def test_main_does_not_overwrite_on_invalid_upstream_data(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "currencies.json"
            original = b'[{"preserved": true}]\n'
            path.write_bytes(original)

            with mock.patch.object(updater, "SAVE_PATH", path), mock.patch.object(
                updater, "fetch_currencies", return_value=[]
            ):
                result = updater.main()

            self.assertEqual(1, result)
            self.assertEqual(original, path.read_bytes())


if __name__ == "__main__":
    unittest.main()
