/** The mandatory final line ending is stored, but is not an editable blank row. */
export function templateBody(content: string): string {
  return content.replace(/\r?\n$/u, '');
}

/** Preserve intentional blank lines and the document's line-ending convention. */
export function terminateTemplate(content: string): string {
  if (content.endsWith('\n')) return content;
  return content + (content.endsWith('\r') ? '\n' : templateLineEnding(content));
}

export function templateLineEnding(content: string): '\r\n' | '\n' {
  const crlf = content.match(/\r\n/gu)?.length ?? 0;
  const lf = (content.match(/\n/gu)?.length ?? 0) - crlf;
  return crlf > lf ? '\r\n' : '\n';
}

/** Editor documents exclude exactly one required terminator. */
export function storeTemplateBody(body: string, ending = templateLineEnding(body)): string {
  return body + ending;
}
