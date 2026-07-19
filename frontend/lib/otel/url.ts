const TRACE_URL_BASE = "http://homebox.invalid";

/**
 * Remove URL components that may contain credentials, tenant identifiers, or
 * other request secrets before recording a URL in telemetry.
 */
export function sanitizeTraceURL(rawURL: string): string {
  try {
    const absolute = /^[a-z][a-z\d+.-]*:\/\//i.test(rawURL);
    const parsed = new URL(rawURL, TRACE_URL_BASE);
    const path = parsed.pathname || "/";

    return absolute ? `${parsed.origin}${path}` : path;
  } catch {
    return "[invalid-url]";
  }
}
