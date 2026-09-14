export interface ScheduleTimezonePreview {
  timezone: string;
  at: string;
  local_time: string;
  abbreviation: string;
  offset_seconds: number;
}

export function parseScheduleTimezonePreview(value: unknown): ScheduleTimezonePreview {
  if (typeof value !== 'object' || value === null) throw new Error('Invalid timezone preview');
  const item = value as Record<string, unknown>;
  if (
    typeof item.timezone !== 'string' ||
    typeof item.at !== 'string' ||
    typeof item.local_time !== 'string' ||
    typeof item.abbreviation !== 'string' ||
    typeof item.offset_seconds !== 'number' ||
    !Number.isInteger(item.offset_seconds) ||
    !Number.isFinite(Date.parse(item.at)) ||
    !Number.isFinite(Date.parse(item.local_time))
  )
    throw new Error('Invalid timezone preview');
  return {
    timezone: item.timezone,
    at: item.at,
    local_time: item.local_time,
    abbreviation: item.abbreviation,
    offset_seconds: item.offset_seconds,
  };
}

/** Browser names aid discovery; only the scheduler can confirm support. */
export function timezoneSuggestions(current: string): string[] {
  const zones = Intl.supportedValuesOf('timeZone');
  return [...new Set(['UTC', ...zones, ...(current ? [current] : [])])].sort();
}

export function timezoneLocalLabel(preview: ScheduleTimezonePreview): string {
  // Format the server's civil fields, without consulting the browser's timezone rules.
  const civil = new Date(`${preview.local_time.slice(0, 19)}Z`);
  const date = new Intl.DateTimeFormat(undefined, {
    timeZone: 'UTC',
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).format(civil);
  const offset = Math.abs(preview.offset_seconds);
  const hours = String(Math.floor(offset / 3600)).padStart(2, '0');
  const minutes = String(Math.floor((offset % 3600) / 60)).padStart(2, '0');
  const zone =
    offset === 0 ? 'UTC' : `UTC${preview.offset_seconds < 0 ? '-' : '+'}${hours}:${minutes}`;
  return `${date} (${zone})`;
}
