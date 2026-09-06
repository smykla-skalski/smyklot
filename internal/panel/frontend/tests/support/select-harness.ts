import { mount } from 'svelte';
import SelectHarness from './SelectHarness.svelte';
const target = document.createElement('div');
target.id = 'select-test-fixture';
target.style.cssText = 'position:fixed;inset:0;z-index:50;background:var(--canvas);overflow:auto';
document.querySelector('.app-shell')!.append(target);
mount(SelectHarness, { target });
