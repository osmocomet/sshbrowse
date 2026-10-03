export function errorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  return message.replace(/^(?:Error|RuntimeError):\s*/, "");
}
