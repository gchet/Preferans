import {wholeWhists} from './settlement';

export interface HistoryEntry {
  id: string;
  names: string[];
  playerIDs?: string[];
  bots?: boolean[];
  results: number[];
  round: number;
  finishedAt: number;
  startedAt: number;
}
export interface HistoryTotal { name: string; whists: number; parties: number; bot: boolean }

export function historyTotals(entries: HistoryEntry[]): HistoryTotal[] {
  const totals = new Map<string, HistoryTotal>();
  const newestFirst = [...entries].sort((a,b) => b.finishedAt-a.finishedAt);
  const latestNames = new Map<string, string>();
  // Device IDs track renames, while names combine a player's different devices.
  for (const entry of newestFirst) {
    entry.names.forEach((name, seat) => {
      const id = entry.playerIDs?.[seat];
      if (id && !entry.bots?.[seat] && !latestNames.has(id)) latestNames.set(id, name.trim());
    });
  }
  // Round each party before adding its scores.
  for (const entry of newestFirst) {
    const results = wholeWhists(entry.results || []);
    entry.names.forEach((name, seat) => {
      if (results[seat] == null) return;
      const bot = entry.bots?.[seat] ?? false;
      const id = entry.playerIDs?.[seat];
      const displayName = (!bot && id ? latestNames.get(id) : name)?.trim() || name.trim();
      const key = `${bot ? 'bot' : 'player'}:${displayName.normalize('NFC').toLocaleLowerCase('ru')}`;
      const total = totals.get(key) || {name:displayName, whists:0, parties:0, bot};
      total.whists += results[seat];
      total.parties++;
      totals.set(key, total);
    });
  }
  return [...totals.values()].sort((a,b) => b.whists-a.whists || a.name.localeCompare(b.name));
}
