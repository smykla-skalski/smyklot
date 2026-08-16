// @vitest-environment jsdom
import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import Plate from '../src/lib/components/Plate.svelte';

describe('Plate [Component]', () => {
  it('renders a labelled alarm surface without an empty body', () => {
    const { container } = render(Plate, { label: 'Problem', tone: 'alarm' });

    expect(screen.getByRole('heading', { name: 'Problem', level: 2 })).toBeTruthy();
    expect(container.querySelector('.plate-alarm')).toBeTruthy();
    expect(container.querySelector('.plate-header-only')).toBeTruthy();
    expect(container.querySelector('.plate-body')).toBeNull();
  });
});
