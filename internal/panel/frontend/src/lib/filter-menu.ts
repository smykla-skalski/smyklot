export type FilterTone = 'default' | 'on' | 'off' | 'valid' | 'missing' | 'invalid' | 'bypassed';

export interface FilterOption {
  value: string;
  label: string;
  description?: string;
  exclusive?: boolean;
  tone?: FilterTone;
}

export interface FilterSection {
  label?: string;
  options: readonly FilterOption[];
}

export function updateFilterSelection(
  selected: readonly string[],
  option: FilterOption,
  options: readonly FilterOption[],
  multiple: boolean,
  fallbackValue?: string,
): string[] {
  if (!multiple || option.exclusive === true) return [option.value];

  const exclusiveValues = new Set(
    options.filter((candidate) => candidate.exclusive === true).map((candidate) => candidate.value),
  );
  const next = selected.filter((value) => !exclusiveValues.has(value));
  const index = next.indexOf(option.value);
  if (index === -1) next.push(option.value);
  else next.splice(index, 1);

  return next.length === 0 && fallbackValue !== undefined ? [fallbackValue] : next;
}
