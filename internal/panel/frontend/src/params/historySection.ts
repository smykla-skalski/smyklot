import type { ParamMatcher } from '@sveltejs/kit';

export const match: ParamMatcher = (param) => param === 'audit' || param === 'failures';
