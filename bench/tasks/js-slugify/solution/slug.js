export function slugify(title, maxLength = 60) {
  let s = title
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  if (s.length <= maxLength) return s;
  const cut = s.lastIndexOf("-", maxLength);
  s = cut > 0 ? s.slice(0, cut) : s.slice(0, maxLength);
  return s.replace(/-+$/, "");
}
