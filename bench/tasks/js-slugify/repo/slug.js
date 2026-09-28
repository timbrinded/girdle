/**
 * Turn a title into a URL slug.
 *
 * - Lowercase ASCII letters and digits are kept. Uppercase letters are lowercased.
 * - Accented Latin letters become their plain form: "é" becomes "e", "Ñ" becomes "n".
 * - Any run of other characters (spaces, punctuation, symbols) becomes a single hyphen.
 * - The slug never starts or ends with a hyphen.
 * - The slug is at most `maxLength` characters (default 60). When it is too long,
 *   cut it at the last hyphen that keeps it within the limit; if there is no such
 *   hyphen, cut it at exactly `maxLength` characters. Never leave a trailing hyphen.
 * - An empty result is returned as "".
 */
export function slugify(title, maxLength = 60) {
  throw new Error("not implemented");
}
