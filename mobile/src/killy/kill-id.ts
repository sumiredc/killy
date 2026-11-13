/**
 * A new kill ID: a lower-case UUID v4 (version digit 4, variant bits 10), the only form the Killy
 * server accepts. Made from 16 random bytes so the form does not depend on what a library returns.
 */
export function newKillId(randomBytes: (n: number) => Uint8Array): string {
  const b = randomBytes(16);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const hex = Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
