import type { ParamMatcher } from '@sveltejs/kit';

const views = new Set(['settings', 'repositories', 'users', 'invitations', 'history']);

export const match: ParamMatcher = (param) => views.has(param);
