<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SchedulesView from '#lib/components/SchedulesView.svelte';
  import type { QueuePolicy, ScheduleProfile, ScheduleRequest } from '#lib/types.js';
  import { stubApi } from '../support/api.js';

  const profile: ScheduleProfile = {
    id: 'europe-hours',
    name: 'Europe business hours',
    timezone: 'Europe/Warsaw',
    system: false,
    revision: 4,
    affected_installations: 3,
    affected_items: 8,
    affected_policies: 2,
    windows: [1, 2, 3, 4, 5].map((weekday) => ({
      weekday,
      start_minute: 540,
      end_minute: 1020,
    })),
    exceptions: [{ date: '2026-12-25', closed: true }],
  };
  const policy: QueuePolicy = {
    kind: 'sync_scan',
    enabled: true,
    cadence: 21_600_000_000_000,
    profile_id: profile.id,
    default_priority: 'normal',
    retry_delay: 300_000_000_000,
    approval_ttl: 7_200_000_000_000,
    revision: 3,
    updated_at: '2026-08-24T12:00:00Z',
  };
  const request: ScheduleRequest = {
    id: 'request:17',
    target_id: 'github:installation:42',
    kind: 'sync_scan',
    state: 'pending',
    base_revision: 3,
    profile_id: profile.id,
    cadence: 10_800_000_000_000,
    default_priority: 'high',
    reason: 'Keep the plan current during the release window',
    requested_by: 'github:user:7',
    revision: 1,
    created_at: '2026-08-24T12:00:00Z',
    updated_at: '2026-08-24T12:00:00Z',
  };
  const api = stubApi({
    fetchRootScheduleProfiles: async () => [profile],
    fetchRootJobPolicies: async () => ({
      policies: [policy],
      policy_set: {
        current: [policy],
        deployment_defaults: [policy],
        overrides: [],
        effective: [policy],
      },
      statuses: [
        {
          kind: 'sync_scan',
          last_run_at: '2026-08-24T11:00:00Z',
          last_state: 'succeeded',
          next_eligibility_at: '2026-08-24T15:00:00Z',
          estimated_start_at: '2026-08-24T15:02:00Z',
          work_ahead: 2,
          current_state: 'scheduled',
        },
      ],
    }),
    fetchRootScheduleRequests: async () => [request],
  });

  const { Story } = defineMeta({
    title: 'Views/SchedulesView',
    component: SchedulesView,
    args: { api, rootRole: 'Super Root' },
  });
</script>

<Story name="Root schedules" />
