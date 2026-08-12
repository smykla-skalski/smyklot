import { panelUrl } from './base';

type PanelServiceWorkerContainer = Pick<ServiceWorkerContainer, 'register'>;

/** Register production builds only; a dev worker would outlive Vite and mask source changes. */
export async function registerPanelServiceWorker(
  base: string,
  buildVersion: string | null,
  serviceWorkers: PanelServiceWorkerContainer | null = browserServiceWorkers(),
): Promise<ServiceWorkerRegistration | null> {
  if (buildVersion === null || serviceWorkers === null) {
    return null;
  }

  return serviceWorkers.register(panelUrl(base, '/sw.js'), {
    scope: panelUrl(base, '/'),
    updateViaCache: 'none',
  });
}

function browserServiceWorkers(): PanelServiceWorkerContainer | null {
  return typeof navigator === 'undefined' || !('serviceWorker' in navigator)
    ? null
    : navigator.serviceWorker;
}
