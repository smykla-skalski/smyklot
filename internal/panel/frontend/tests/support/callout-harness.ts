import { mount } from 'svelte';

import CalloutHarness from './CalloutHarness.svelte';

const target = document.createElement('div');
target.id = 'callout-test-fixture';
target.style.cssText = 'position:fixed;inset:0;z-index:9999;background:var(--canvas);overflow:auto';
document.querySelector('.app-shell')!.append(target);
mount(CalloutHarness, { target });
