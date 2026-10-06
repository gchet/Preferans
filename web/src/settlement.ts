// Exact numerators stay unchanged in saves and on the wire.
// Largest remainders give whole whists with zero total. Seat order breaks ties.
export function wholeWhists(numerators: number[]): number[] {
  const n = numerators.length;
  if (!n) return [];
  const whole = numerators.map(value => Math.floor(value / n));
  const order = numerators.map((value, seat) => ({seat, remainder: value - whole[seat] * n}))
    .sort((a, b) => b.remainder - a.remainder || a.seat - b.seat);
  const missing = -whole.reduce((sum, value) => sum + value, 0);
  for (let i = 0; i < missing; i++) whole[order[i].seat]++;
  return whole;
}
