import { parseScheduleTimezonePreview, type ScheduleTimezonePreview } from './schedule-timezone';
export interface LocalTimeResolution {
  timezone: string;
  local_time: string;
  options: ScheduleTimezonePreview[];
}
export function parseLocalTimeResolution(value: unknown): LocalTimeResolution {
  if (typeof value !== 'object' || value === null) throw new Error('Invalid local time resolution');
  const record = value as Record<string, unknown>;
  if (
    typeof record.timezone !== 'string' ||
    typeof record.local_time !== 'string' ||
    !Array.isArray(record.options)
  )
    throw new Error('Invalid local time resolution');
  return {
    timezone: record.timezone,
    local_time: record.local_time,
    options: record.options.map(parseScheduleTimezonePreview),
  };
}
