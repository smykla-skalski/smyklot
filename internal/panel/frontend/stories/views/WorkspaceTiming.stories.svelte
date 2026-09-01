<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import WorkspaceTiming from '#lib/components/WorkspaceTiming.svelte';
  import type { QueuePolicy, ScheduleProfile, ScheduleRequest } from '#lib/types.js';
  import { stubApi } from '../support/api.js';

  const alwaysOpen: ScheduleProfile = {
    id: 'always-open',
    name: 'Always Open',
    timezone: 'UTC',
    system: true,
    revision: 1,
    windows: Array.from({ length: 7 }, (_, weekday) => ({
      weekday,
      start_minute: 0,
      end_minute: 1440,
    })),
    exceptions: [],
  };
  const europeHours: ScheduleProfile = {
    id: 'europe-hours',
    name: 'Europe business hours',
    timezone: 'Europe/Warsaw',
    system: false,
    revision: 4,
    windows: [1, 2, 3, 4, 5].map((weekday) => ({
      weekday,
      start_minute: 540,
      end_minute: 1020,
    })),
    exceptions: [{ date: '2026-12-25', closed: true }],
  };

  function policy(profile: ScheduleProfile): QueuePolicy {
    return {
      kind: 'sync_scan',
      enabled: true,
      cadence: 21_600_000_000_000,
      profile_id: profile.id,
      default_priority: 'normal',
      retry_delay: 300_000_000_000,
      revision: 3,
      updated_at: '2026-08-24T12:00:00Z',
    };
  }

  const waiting: ScheduleRequest = {
    id: 'request:17',
    target_id: 'github:workspace:42',
    kind: 'sync_scan',
    state: 'pending',
    base_revision: 3,
    profile_id: europeHours.id,
    cadence: 10_800_000_000_000,
    default_priority: 'high',
    reason: 'Keep the plan current during the release window',
    requested_by: 'github:user:7',
    revision: 1,
    created_at: '2026-08-24T12:00:00Z',
    updated_at: '2026-08-24T12:00:00Z',
  };

  function schedules(profile: ScheduleProfile, requests: ScheduleRequest[] = []) {
    const only = policy(profile);
    return stubApi({
      fetchTargetSchedules: async () => ({
        policies: {
          current: [only],
          deployment_defaults: [only],
          overrides: [],
          effective: [only],
        },
        profiles: [profile],
        statuses: [],
      }),
      fetchTargetScheduleRequests: async () => requests,
    });
  }

  const { Story } = defineMeta({
    title: 'Views/WorkspaceTiming',
    component: WorkspaceTiming,
    args: {
      api: schedules(alwaysOpen),
      targetId: 'github:workspace:42',
      canRequest: true,
    },
  });
</script>

<!--
  The answer read from the windows rather than from the profile's name: seven days from
  midnight to midnight IS around the clock, whatever the operators called it.
-->
<Story name="Around the clock" />

<!-- A window with hours in it says which, and where those hours are. -->
<Story name="Named hours" args={{ api: schedules(europeHours) }} />

<!-- A member who cannot ask reads the answer and is offered nothing to press. -->
<Story name="Read only" args={{ canRequest: false }} />

<!-- An ask already with the operators stays on the page until they decide. -->
<Story name="Waiting on a decision" args={{ api: schedules(alwaysOpen, [waiting]) }} />
