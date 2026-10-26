// Keep command limits and public intent IDs portable to mini-program engines
// that do not expose the browser TextEncoder global. Match standard UTF-8,
// including replacement of unpaired UTF-16 surrogates with U+FFFD.
function codePoint(character: string) {
  const value = character.codePointAt(0)!;
  return value >= 0xd800 && value <= 0xdfff ? 0xfffd : value;
}
export function utf8ByteLength(value: string) {
  let length = 0;
  for (const character of value) {
    const point = codePoint(character);
    length += point < 0x80 ? 1 : point < 0x800 ? 2 : point < 0x10000 ? 3 : 4;
  }
  return length;
}
export function utf8Bytes(value: string) {
  const result = new Uint8Array(utf8ByteLength(value)); let at = 0;
  for (const character of value) {
    const point = codePoint(character);
    if (point < 0x80) result[at++] = point;
    else if (point < 0x800) { result[at++] = 0xc0 | point >> 6; result[at++] = 0x80 | point & 0x3f; }
    else if (point < 0x10000) { result[at++] = 0xe0 | point >> 12; result[at++] = 0x80 | point >> 6 & 0x3f; result[at++] = 0x80 | point & 0x3f; }
    else { result[at++] = 0xf0 | point >> 18; result[at++] = 0x80 | point >> 12 & 0x3f; result[at++] = 0x80 | point >> 6 & 0x3f; result[at++] = 0x80 | point & 0x3f; }
  }
  return result;
}
