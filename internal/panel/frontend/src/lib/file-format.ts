/** File capabilities shared by adjustment validation and formatting controls. */
export type FileFormat = 'json' | 'jsonc' | 'yaml' | 'toml' | 'markdown';

const FORMATS: Readonly<Record<string, FileFormat>> = {
  json: 'json',
  jsonc: 'jsonc',
  yaml: 'yaml',
  yml: 'yaml',
  toml: 'toml',
  md: 'markdown',
  markdown: 'markdown',
};

/** Keep these extensions aligned with filemerge.formatOf and isMarkdown. */
export function fileFormat(path: string): FileFormat | null {
  const extension = /\.([^./]+)$/u.exec(path)?.[1]?.toLowerCase();
  return extension !== undefined && Object.hasOwn(FORMATS, extension) ? FORMATS[extension]! : null;
}

export function isStructuredFile(path: string): boolean {
  const format = fileFormat(path);
  return format !== null && format !== 'markdown';
}
