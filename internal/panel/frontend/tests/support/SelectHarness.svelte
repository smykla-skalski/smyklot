<script lang="ts">
  import Select, { type SelectValue } from '#lib/components/Select.svelte';
  import Button from '#lib/components/Button.svelte';
  let value = $state<string | number | null | undefined>(1);
  let requiredValue = $state<string | undefined>(undefined);
  let submitted = $state('');
  const options = [
    { value: 1, label: 'Numeric one' },
    { value: '1', label: 'Text one' },
    { value: '', label: 'Empty value' },
    { value: null, label: 'No override' },
    { value: 'disabled', label: 'Unavailable option', disabled: true },
    { value: 'z', label: 'Zulu' },
  ];
</script>

<form
  id="picker-form"
  class="form-stack"
  onsubmit={(event) => {
    event.preventDefault();
    submitted = JSON.stringify([...new FormData(event.currentTarget)]);
  }}
>
  <label class="form-field"
    ><span class="form-label">Typed choice</span>
    <Select aria-label="Typed choice" name="choice" bind:value {options} />
  </label>
  <output aria-label="Typed result">{value === null ? 'null' : `${typeof value}:${value}`}</output>
  <label class="form-field"
    ><span class="form-label">Required choice</span>
    <Select
      aria-label="Required choice"
      name="required"
      required
      bind:value={requiredValue}
      options={[{ value: 'yes', label: 'Confirmed' }]}
    />
  </label>
  <Select aria-label="Disabled picker" value={'disabled' as SelectValue} {options} disabled />
  <Select aria-label="Saved unavailable value" value={'retired' as SelectValue} {options} />
  <Select aria-label="Selected disabled option" value={'disabled' as SelectValue} {options} />
  <div class="form-actions">
    <Button type="submit">Submit</Button><Button type="reset">Reset</Button>
  </div>
  <output aria-label="Form result">{submitted}</output>
</form>

<style>
  form {
    max-inline-size: 30rem;
    margin-inline: auto;
    padding: var(--space-4);
  }
  .form-actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
