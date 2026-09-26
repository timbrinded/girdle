export function parse(text) {
  const rows = [];
  let row = [];
  let field = "";
  let i = 0;
  const n = text.length;
  if (n === 0) return rows;
  let atFieldStart = true;
  while (i < n) {
    const c = text[i];
    if (atFieldStart && c === '"') {
      i++;
      for (;;) {
        if (i >= n) throw new Error("unterminated quote");
        if (text[i] === '"') {
          if (text[i + 1] === '"') { field += '"'; i += 2; continue; }
          i++;
          break;
        }
        field += text[i++];
      }
      if (i < n && text[i] !== "," && text[i] !== "\n" && !(text[i] === "\r" && text[i + 1] === "\n")) {
        throw new Error("unexpected character after closing quote");
      }
      atFieldStart = false;
      continue;
    }
    if (c === ",") { row.push(field); field = ""; atFieldStart = true; i++; continue; }
    if (c === "\n" || (c === "\r" && text[i + 1] === "\n")) {
      row.push(field); rows.push(row); row = []; field = ""; atFieldStart = true;
      i += c === "\r" ? 2 : 1;
      continue;
    }
    field += c; atFieldStart = false; i++;
  }
  if (!(atFieldStart && row.length === 0 && field === "" && (text.endsWith("\n")))) {
    row.push(field); rows.push(row);
  }
  return rows;
}
