// What's typed, held outside Preact so everything that goes to Go goes
// after it: part 2's input protocol on the page. Typing is sent as a
// numbered `input` a moment after you stop; any other action sends it
// first, so a switch, a submit or leaving the tab never loses it (#141).
// Go's Input event carries Ack, the newest number it has taken: the page
// takes Go's text only when that's its newest action and nothing's been
// typed since.
import { takes } from "./input.ts";

interface Rec {
  n: number; // the last action number sent for this input
  text: string; // the page's own text
  pending: boolean; // typed and not sent yet
}

export class Typing {
  private recs = new Map<string, Rec>();
  private timer: ReturnType<typeof setTimeout> | undefined;
  private send_: ((o: object, secret?: boolean) => void) | undefined;
  private next = 0;
  private queue: [object, boolean][] = []; // actions taken before Kiln ran, in order

  constructor(private wait = 300) {}

  // ready starts sending, through send, once Kiln's loop runs: first the
  // actions taken before, in order, then typing held until now. Typed
  // while it loaded, the text was the only input there was (""); if Kiln
  // opened with a character showing (autoconnect), it's that character's.
  ready(send: (o: object, secret?: boolean) => void, home = ""): void {
    const idle = this.recs.get("");
    if (home && idle?.pending && !this.recs.get(home)?.pending) {
      this.recs.delete("");
      this.recs.set(home, idle);
    }
    this.send_ = send;
    for (const [o, secret] of this.queue.splice(0)) send(o, secret);
    this.flush();
  }

  // out sends o, or before Kiln runs keeps it for ready.
  private out(o: object, secret = false): void {
    if (this.send_) this.send_(o, secret);
    else this.queue.push([o, secret]);
  }

  private rec(key: string): Rec {
    let r = this.recs.get(key);
    if (!r) this.recs.set(key, (r = { n: 0, text: "", pending: false }));
    return r;
  }

  // edit records text typed in key's input, sent after the wait.
  edit(key: string, text: string): void {
    const r = this.rec(key);
    r.text = text;
    r.pending = true;
    clearTimeout(this.timer);
    this.timer = setTimeout(() => this.flush(), this.wait);
  }

  // flush sends what's typed and not sent, now. Before Kiln runs, typing
  // waits for it, unless another action needs it to go first (now).
  flush(now = false): void {
    clearTimeout(this.timer);
    if (!this.send_ && !now) return;
    for (const [Key, r] of this.recs) {
      if (!r.pending) continue;
      r.pending = false;
      r.n = ++this.next;
      this.out({ Do: "input", Key, Text: r.text, N: r.n });
    }
  }

  // act sends o after what's typed.
  act(o: object): void {
    this.flush(true);
    this.out(o);
  }

  // send sends a numbered action for key's input (submit, older, newer),
  // after what's typed.
  send(key: string, o: { Do: string; Text?: string }): void {
    this.flush();
    if (!this.send_) return;
    const r = this.rec(key);
    r.n = ++this.next;
    this.send_({ ...o, Key: key, N: r.n });
  }

  // submit sends key's text, and clears it here at once as the TUI does;
  // Go's answer puts it back if it was refused. Before Kiln runs there's
  // no one to send it to, and the text stays.
  submit(key: string, text: string): void {
    if (!this.send_) return;
    this.send(key, { Do: "submit", Text: text });
    this.rec(key).text = "";
  }

  // password sends key's password: a submit, never recorded (send's
  // secret) and never held here.
  password(key: string, text: string): void {
    this.flush();
    if (!this.send_) return;
    const r = this.rec(key);
    r.n = ++this.next;
    this.send_({ Do: "submit", Key: key, Text: text, N: r.n }, true);
  }

  // saw notes an action number Go has taken, so the page's next number is
  // past it: whatever else numbered actions (a test, an older page), the
  // page's typing is never taken for older than Go's text.
  saw(ack: number): void {
    this.next = Math.max(this.next, ack);
  }

  // takes reports whether to show Go's text for key, acked to ack.
  takes(key: string, ack: number): boolean {
    const r = this.recs.get(key);
    return takes(r && { n: r.n, dirty: r.pending }, ack);
  }

  // text is the page's own text for key.
  text(key: string): string | undefined {
    return this.recs.get(key)?.text;
  }

  // adopt makes Go's text the page's own, once taken.
  adopt(key: string, text: string): void {
    this.rec(key).text = text;
  }
}
