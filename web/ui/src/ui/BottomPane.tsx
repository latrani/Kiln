import { targetKey } from "../editor.ts";
import { historyShows, logShows } from "../store.ts";
import type { State, Store } from "../store.ts";
import { roleClass } from "../theme.ts";
import { EditorForm } from "./EditorForm.tsx";
import { HistoryPanel } from "./HistoryPanel.tsx";
import { Input } from "./Input.tsx";
import { LogBar } from "./LogBar.tsx";
import { PasswordForm } from "./PasswordForm.tsx";
import { PickerForm } from "./PickerForm.tsx";

// BottomPane is under the scrollback: the input, or the password form,
// or with the picker open its filter or an editor over it (in the form's
// area, even on a world's overview); in log mode its hints or prompt
// (LogBar). A world's overview otherwise has none, as in the TUI: the
// overview has the pane. With the history panel up, the pane takes its
// most, the panel over the input.
// Each character gets a password form of its own, so what's typed in one
// never goes to another's server.
export function BottomPane({ state, store }: { state: State; store: Store }) {
  const overview = state.active.startsWith("=");
  const password = state.chars[state.active] && state.inputs[state.active]?.NeedPW;
  if (state.picker) {
    return (
      <div class={`bottom form ${roleClass("form")}`}>
        {state.picker.edit ? <EditorForm key={targetKey(state.picker.edit.target)} state={state} store={store} /> : <PickerForm state={state} store={store} />}
      </div>
    );
  }
  if (logShows(state, state.active)) {
    return (
      <div class="bottom form">
        <LogBar state={state} store={store} />
      </div>
    );
  }
  const history = historyShows(state);
  return (
    <div class={`bottom ${roleClass("input")}` + (history ? " histories" : "")}>
      {history && <HistoryPanel state={state} />}
      {!overview && (password ? <PasswordForm key={state.active} state={state} store={store} /> : <Input state={state} store={store} />)}
    </div>
  );
}
