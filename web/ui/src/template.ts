// fill puts slots into a template's {{name}}s, escaped for HTML. A slot
// the template names but isn't given is an error, so a typo can't ship
// as "undefined".
export function fill(template: string, slots: Record<string, string>): string {
  const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  return template.replace(/\{\{(\w+)\}\}/g, (_, k: string) => {
    if (!(k in slots)) throw new Error(`index.html: no slot ${k}`); // a developer's message
    return esc(slots[k]);
  });
}
