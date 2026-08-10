<script module lang="ts">
  export type IconName =
    | 'admin'
    | 'ban'
    | 'branch'
    | 'chevron-down'
    | 'chevron-left'
    | 'chevron-right'
    | 'chevron-up'
    | 'chevrons-up-down'
    | 'close'
    | 'editor'
    | 'failure'
    | 'file'
    | 'file-bypassed'
    | 'file-invalid'
    | 'file-missing'
    | 'file-valid'
    | 'filter'
    | 'globe'
    | 'history'
    | 'info'
    | 'lock'
    | 'more'
    | 'moon'
    | 'minus-circle'
    | 'no-access'
    | 'notifications'
    | 'organization'
    | 'owner'
    | 'pending'
    | 'plus'
    | 'repositories'
    | 'refresh'
    | 'search'
    | 'settings'
    | 'shield'
    | 'shield-slash'
    | 'sidebar'
    | 'sign-out'
    | 'sort'
    | 'success'
    | 'sun'
    | 'sun-moon'
    | 'system'
    | 'trash'
    | 'user'
    | 'user-plus'
    | 'users'
    | 'viewer'
    | 'warning';
</script>

<script lang="ts">
  const {
    name,
    size = 20,
    strokeWidth = 1.75,
    class: className,
  }: {
    name: IconName;
    size?: number;
    strokeWidth?: number;
    class?: string;
  } = $props();
</script>

<svg
  class={className}
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={strokeWidth}
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if name === 'settings'}
    <path d="M4 7h10M18 7h2M4 17h2M10 17h10M4 12h4M12 12h8" />
    <circle cx="16" cy="7" r="2" />
    <circle cx="8" cy="17" r="2" />
    <circle cx="10" cy="12" r="2" />
  {:else if name === 'repositories'}
    <path d="M4 5.5h6l2 2h8v11a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />
    <path d="M8 13h8M8 16.5h5" />
  {:else if name === 'users'}
    <circle cx="9" cy="8" r="3" />
    <circle cx="17" cy="9" r="2.5" />
    <path d="M3.5 19c.5-3.2 2.4-5 5.5-5s5 1.8 5.5 5M14 14.5c3.5-.5 5.6 1 6.5 4.5" />
  {:else if name === 'history'}
    <path d="M4 5v5h5" />
    <path d="M5.4 15.5A8 8 0 1 0 6 7.2L4 10" />
    <path d="M12 8v4l2.75 1.75" />
  {:else if name === 'system'}
    <rect x="3" y="4" width="18" height="13" rx="2" />
    <path d="M8 21h8M12 17v4" />
  {:else if name === 'search'}
    <circle cx="10.5" cy="10.5" r="6.5" />
    <path d="m15.5 15.5 4.5 4.5" />
  {:else if name === 'filter'}
    <path d="M3.5 5h17l-6.7 7.3v5.4l-3.6 1.8v-7.2z" />
  {:else if name === 'globe'}
    <circle cx="12" cy="12" r="9" />
    <path
      d="M3 12h18M12 3c2.4 2.4 3.6 5.4 3.6 9S14.4 18.6 12 21M12 3C9.6 5.4 8.4 8.4 8.4 12s1.2 6.6 3.6 9"
    />
  {:else if name === 'organization'}
    <path d="M4 21V7l8-4 8 4v14M8 21v-4h8v4" />
    <path d="M8 9h1M11.5 9h1M15 9h1M8 13h1M11.5 13h1M15 13h1" />
  {:else if name === 'user'}
    <circle cx="12" cy="8" r="4" />
    <path d="M4.5 21c.6-4.3 3.1-6.5 7.5-6.5s6.9 2.2 7.5 6.5" />
  {:else if name === 'branch'}
    <circle cx="7" cy="5" r="2" />
    <circle cx="17" cy="8" r="2" />
    <circle cx="7" cy="19" r="2" />
    <path d="M7 7v10M9 8h3a5 5 0 0 1 5 5v-3" />
  {:else if name === 'lock'}
    <rect x="5" y="10" width="14" height="10" rx="2" />
    <path d="M8 10V7a4 4 0 0 1 8 0v3" />
  {:else if name === 'user-plus'}
    <circle cx="9" cy="8" r="3" />
    <path d="M3.5 19c.5-3.2 2.4-5 5.5-5 2.2 0 3.8.9 4.7 2.4M18 8v6M15 11h6" />
  {:else if name === 'more'}
    <circle cx="12" cy="5" r="1.65" fill="currentColor" stroke="none" />
    <circle cx="12" cy="12" r="1.65" fill="currentColor" stroke="none" />
    <circle cx="12" cy="19" r="1.65" fill="currentColor" stroke="none" />
  {:else if name === 'trash'}
    <path d="M4 7h16M9 3h6l1 4H8zM6.5 7l.8 14h9.4l.8-14M10 11v6M14 11v6" />
  {:else if name === 'ban'}
    <circle cx="12" cy="12" r="9" />
    <path d="m6 6 12 12" />
  {:else if name === 'minus-circle'}
    <circle cx="12" cy="12" r="9" />
    <path d="M8 12h8" />
  {:else if name === 'refresh'}
    <path d="M20 7v5h-5M4 17v-5h5" />
    <path d="M6.1 8.2A7 7 0 0 1 18.6 7L20 12M4 12l1.4 5A7 7 0 0 0 17.9 15.8" />
  {:else if name === 'shield'}
    <path d="M12 3 19 6v5c0 4.7-2.8 8.2-7 10-4.2-1.8-7-5.3-7-10V6z" />
  {:else if name === 'sun-moon'}
    <path d="M12 8a2.8 2.8 0 0 0 4 4 4 4 0 1 1-4-4" />
    <path
      d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M18.7 5.3l-1.4 1.4M6.7 17.3l-1.4 1.4"
    />
  {:else if name === 'shield-slash'}
    <path
      d="M12 3 19 6v5c0 2.1-.6 4-1.7 5.6M14.8 19.3c-.8.7-1.8 1.2-2.8 1.7-4.2-1.8-7-5.3-7-10V6l3.1-1.3"
    />
    <path d="M3.5 3.5 20.5 20.5" />
  {:else if name === 'sign-out'}
    <path d="M10 5H5v14h5M14 8l4 4-4 4M8 12h10" />
  {:else if name === 'sun'}
    <circle cx="12" cy="12" r="3.5" />
    <path
      d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M18.7 5.3l-1.4 1.4M6.7 17.3l-1.4 1.4"
    />
  {:else if name === 'moon'}
    <path d="M20 15.2A8 8 0 0 1 8.8 4a8.1 8.1 0 1 0 11.2 11.2Z" />
  {:else if name === 'success'}
    <circle cx="12" cy="12" r="9" />
    <path d="m8 12 2.5 2.5L16.5 9" />
  {:else if name === 'pending'}
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  {:else if name === 'notifications'}
    <path d="M6 9a6 6 0 0 1 12 0c0 6 2.5 6.5 2.5 6.5h-17S6 15 6 9" />
    <path d="M9.5 19a2.8 2.8 0 0 0 5 0" />
  {:else if name === 'warning'}
    <path d="M12 3 2.8 20h18.4z" />
    <path d="M12 9v4M12 17h.01" />
  {:else if name === 'failure'}
    <circle cx="12" cy="12" r="9" />
    <path d="m9 9 6 6M15 9l-6 6" />
  {:else if name === 'info'}
    <circle cx="12" cy="12" r="9" />
    <path d="M12 11v6M12 7h.01" />
  {:else if name === 'sidebar'}
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <path d="M9 4v16M14 9l3 3-3 3" />
  {:else if name === 'chevron-down'}
    <path d="m7 9 5 5 5-5" />
  {:else if name === 'chevron-left'}
    <path d="m14.5 6-6 6 6 6" />
  {:else if name === 'chevron-right'}
    <path d="m9.5 6 6 6-6 6" />
  {:else if name === 'chevron-up'}
    <path d="m7 14.5 5-5 5 5" />
  {:else if name === 'chevrons-up-down'}
    <path d="m7 15 5 5 5-5M7 9l5-5 5 5" />
  {:else if name === 'close'}
    <path d="m6 6 12 12M18 6 6 18" />
  {:else if name === 'plus'}
    <path d="M12 5v14M5 12h14" />
  {:else if name === 'sort'}
    <path d="m8 9 4-4 4 4M16 15l-4 4-4-4" />
  {:else if name === 'owner'}
    <path d="M12 3 19 6v5c0 4.7-2.8 8.2-7 10-4.2-1.8-7-5.3-7-10V6z" />
    <path d="m9 12 2 2 4-4" />
  {:else if name === 'admin'}
    <circle cx="8" cy="12" r="4" />
    <path d="M12 12h9M17 12v3M20 12v2" />
  {:else if name === 'editor'}
    <path d="m4 20 4.2-1 10.9-10.9-3.2-3.2L5 15.8zM14.8 6l3.2 3.2" />
  {:else if name === 'viewer'}
    <path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6" />
    <circle cx="12" cy="12" r="2.5" />
  {:else if name === 'no-access'}
    <path
      d="M3.5 3.5 20.5 20.5M9.6 6.4A10.8 10.8 0 0 1 12 6c6 0 9.5 6 9.5 6a16 16 0 0 1-2.2 2.9M6.3 7.3A17.2 17.2 0 0 0 2.5 12s3.5 6 9.5 6a10 10 0 0 0 3.2-.5"
    />
  {:else if name === 'file' || name.startsWith('file-')}
    <path d="M6 3h8l4 4v14H6zM14 3v5h4" />
    {#if name === 'file-valid'}
      <path d="m9 14 2 2 4-4" />
    {:else if name === 'file-invalid'}
      <path d="m10 12 4 4M14 12l-4 4" />
    {:else if name === 'file-missing'}
      <path d="M9 14h6" stroke-dasharray="2 2" />
    {:else if name === 'file-bypassed'}
      <path d="M10 12h4l-4 4h4" />
    {/if}
  {/if}
</svg>

<style>
  svg {
    display: block;
    flex: none;
    overflow: visible;
  }
</style>
