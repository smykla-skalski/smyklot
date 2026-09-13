export interface DeliveryRecoveryPreview {
  available: boolean;
  reason: string;
  effect?: string;
  revision: number;
  current_run_id?: number;
}

export interface DeliveryRecoveryRequest {
  expected_run_id: number;
  expected_revision: number;
  request_key: string;
}

export interface DeliveryRecoveryResult {
  run_id: number;
  queue_id: string;
  repeated: boolean;
}

export function recoveryExplanation(reason: string): string {
  const explanations: Record<string, string> = {
    history_unavailable:
      'The saved event is no longer available. Trigger a new event in GitHub if the work is still needed.',
    payload_unavailable:
      'The saved event has expired. Trigger a new event in GitHub if the work is still needed.',
    payload_invalid: 'The saved event cannot be processed. Trigger a new event in GitHub.',
    already_running: 'A run is already queued or processing. Inspect the latest run for progress.',
    already_succeeded: 'The latest run completed successfully. No retry is needed.',
    state_changed:
      'The execution changed while recovery was being checked. Check again for the latest state.',
    access_required:
      'An Admin or Owner can retry this work. Root operators must start an operator visit to this workspace.',
    service_unavailable:
      'Recovery is not available from this service. Check again after the service is updated.',
    repository_unavailable:
      'The workspace or repository is unavailable. Check the GitHub App installation and refresh the workspace catalog.',
    repository_disabled:
      'Processing is disabled for this repository. Enable it in repository settings before retrying.',
    source_changed:
      'The original command has changed or been superseded. Review the current comment in GitHub and issue the command there if needed.',
    configuration_invalid:
      'Repository configuration still has errors. Fix the configuration, then check recovery again.',
    permission_changed:
      'The original command author no longer has the required permission. Ask an authorized person to issue the command in GitHub.',
    service_disabled:
      'This repository is configured to use the GitHub Action. Review its workflow runs in GitHub.',
    no_action:
      'The original event no longer requires processing. Review the current repository or pull request state in GitHub.',
    reauthorization_unavailable:
      'The original CI authorization request no longer meets the current requirements. Review the pull request, its required check and your permissions in GitHub.',
  };
  return (
    explanations[reason] ??
    'Recovery is not available for this event. Check again after reviewing the latest execution.'
  );
}

/** Network failures keep this exact request so checking the result cannot create another run. */
export function recoveryRequest(
  preview: DeliveryRecoveryPreview,
  key: string,
): DeliveryRecoveryRequest {
  if (!preview.available || !preview.current_run_id || preview.revision <= 0) {
    throw new Error('Recovery must be available for a current execution.');
  }
  return {
    expected_run_id: preview.current_run_id,
    expected_revision: preview.revision,
    request_key: key,
  };
}
