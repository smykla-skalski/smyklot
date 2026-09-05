/** The mandatory final line ending is stored, but is not an editable blank row. */
export function templateBody(content: string): string {
  return content.replace(/\r?\n$/u, '');
}

/** Preserve intentional blank lines and the document's line-ending convention. */
export function terminateTemplate(content: string): string {
  if (content.endsWith('\n')) return content;
  const crlf = content.match(/\r\n/gu)?.length ?? 0;
  const lf = (content.match(/\n/gu)?.length ?? 0) - crlf;
  return content + (crlf > lf && !content.endsWith('\r') ? '\r\n' : '\n');
}

/** Editor documents exclude exactly one required terminator. */
export function storeTemplateBody(body: string): string {
  const crlf = body.match(/\r\n/gu)?.length ?? 0;
  const lf = (body.match(/\n/gu)?.length ?? 0) - crlf;
  return body + (crlf > lf ? '\r\n' : '\n');
}
