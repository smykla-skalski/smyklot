import type { ParamMatcher } from '@sveltejs/kit';

export const match: ParamMatcher = (param) => param === 'users' || param === 'invitations';
