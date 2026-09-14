import { tick } from 'svelte';

/** Wait for validation markup, then focus the first editable invalid control in this form. */
export async function focusInvalidControl(containerId: string): Promise<void> {
  await tick();
  const container = document.getElementById(containerId);
  const control = container?.querySelector<HTMLElement>(
    'input[aria-invalid="true"]:not(:disabled):not([readonly]), button[aria-invalid="true"]:not(:disabled), textarea[aria-invalid="true"]:not(:disabled):not([readonly]), select[aria-invalid="true"]:not(:disabled)',
  );
  control?.focus();
}
