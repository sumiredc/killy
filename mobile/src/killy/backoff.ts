const first = 1000;
const most = 30_000;

/**
 * How long to wait before reconnection attempt `attempt` (0 for the first after a drop): 1, 2, 4,
 * 8, 16 and then 30 seconds, plus a random 0 to 25% of that (ARCHITECTURE: SSE rules), so phones
 * do not all come back at once after a Killy server restart. The agent uses the same waits.
 */
export function backoff(attempt: number, random: () => number): number {
  const base = Math.min(most, first * 2 ** attempt);
  return base + base * 0.25 * random();
}
