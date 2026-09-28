/**
 * Parse CSV text (RFC 4180) into an array of rows, each an array of strings.
 *
 * - Fields are separated by commas and records by "\n" or "\r\n".
 * - A field may be wrapped in double quotes. Inside a quoted field, commas,
 *   newlines ("\n" or "\r\n") and doubled quotes ("" meaning ") are literal.
 * - Unquoted fields are returned exactly as written, including spaces.
 * - Empty fields are empty strings: "a,,b" is ["a", "", "b"].
 * - A single trailing newline at the end of the text does not create an
 *   empty last record. Empty text returns [].
 * - A quote that is never closed, or a quoted field followed by anything other
 *   than a comma, newline or end of text, throws an Error.
 */
export function parse(text) {
  throw new Error("not implemented");
}
