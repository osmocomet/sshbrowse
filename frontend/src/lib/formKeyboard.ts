export type FormEnterTarget =
  | { kind: "input"; type: string; hasDatalist?: boolean }
  | { kind: "textarea" }
  | { kind: "button" }
  | null;

export function isComposingKey(event: Pick<KeyboardEvent, "isComposing" | "keyCode">): boolean {
  // WebKit can report the Enter key that ends composition with isComposing
  // false and keyCode 229. Treat both signals as composition confirmation.
  return event.isComposing || event.keyCode === 229;
}

export function shouldSaveOnEnter(
  event: Pick<KeyboardEvent, "key" | "isComposing" | "keyCode">,
  target: FormEnterTarget,
): boolean {
  if (event.key !== "Enter" || isComposingKey(event)) {
    return false;
  }
  return target?.kind === "input" && target.type !== "checkbox" && !target.hasDatalist;
}
