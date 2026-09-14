import { PanelApiError, type PanelApi } from '#lib/api.js';

// This catalogue example covers August 2026 in these three zones. Production
// and the dev mock resolve arbitrary dates through the scheduler's Go database.
// Refuse inputs outside the fixture instead of approximating clock-change rules.
export const resolveQueueTimeFixture: PanelApi['resolveScheduleLocalTime'] = async (
  timezone,
  localTime,
) => {
  const offsets: Record<string, { seconds: number; abbreviation: string; suffix: string }> = {
    UTC: { seconds: 0, abbreviation: 'UTC', suffix: 'Z' },
    'Europe/Warsaw': { seconds: 7200, abbreviation: 'CEST', suffix: '+02:00' },
    'Asia/Tokyo': { seconds: 32400, abbreviation: 'JST', suffix: '+09:00' },
  };
  const zone = offsets[timezone];
  const civil = new Date(`${localTime}:00Z`);
  if (
    !zone ||
    !/^2026-08-\d{2}T\d{2}:\d{2}$/.test(localTime) ||
    !Number.isFinite(civil.getTime()) ||
    civil.toISOString().slice(0, 16) !== localTime
  )
    throw new PanelApiError(
      400,
      'fixture_input',
      'This example covers August 2026 in UTC, Europe/Warsaw and Asia/Tokyo. Use the running panel for other dates and timezones.',
    );
  return {
    timezone,
    local_time: localTime,
    options: [
      {
        timezone,
        at: new Date(civil.getTime() - zone.seconds * 1000).toISOString(),
        local_time: `${localTime}:00${zone.suffix}`,
        offset_seconds: zone.seconds,
        abbreviation: zone.abbreviation,
      },
    ],
  };
};
