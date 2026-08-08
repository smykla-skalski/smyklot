import type { CommandName } from './types';

export interface HelpCommand {
  name: CommandName;
  summary: string;
  example: string;
  aliases: readonly string[];
}

export const HELP_COMMANDS: readonly HelpCommand[] = [
  {
    name: 'approve',
    summary: 'Approve the pull request as a global CODEOWNER',
    example: '/approve',
    aliases: ['lgtm', 'accept'],
  },
  {
    name: 'merge',
    summary: 'Merge with the first method allowed by the repository',
    example: '/merge',
    aliases: [],
  },
  {
    name: 'squash',
    summary: 'Merge all pull request commits as one commit',
    example: '/squash',
    aliases: [],
  },
  {
    name: 'rebase',
    summary: 'Rebase commits onto the base branch and merge',
    example: '/rebase',
    aliases: [],
  },
  {
    name: 'unapprove',
    summary: 'Remove your existing Smyklot approval',
    example: '/unapprove',
    aliases: ['disapprove'],
  },
  {
    name: 'cleanup',
    summary: 'Remove Smyklot reactions, approvals, and comments',
    example: '/cleanup',
    aliases: [],
  },
  {
    name: 'help',
    summary: 'Post a compact command guide on the pull request',
    example: '/help',
    aliases: [],
  },
];

export function filterHelpCommands(query: string): readonly HelpCommand[] {
  const normalized = query.trim().toLocaleLowerCase();
  if (normalized === '') return HELP_COMMANDS;

  return HELP_COMMANDS.filter((command) =>
    [command.name, command.summary, command.example, ...command.aliases].some((value) =>
      value.toLocaleLowerCase().includes(normalized),
    ),
  );
}
