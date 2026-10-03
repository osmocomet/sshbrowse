import * as ConnectionsService from "../../bindings/sshbrowse/internal/app/connections";

export interface SSHConfigCandidate {
  alias: string;
  alreadyImported: boolean;
}

export interface SSHConfigIssue {
  source: string;
  line: number;
  message: string;
}

export interface SSHConfigScan {
  candidates: SSHConfigCandidate[];
  warnings: SSHConfigIssue[];
  shouldPrompt: boolean;
}

export interface SSHConfigImportResult {
  imported: number;
  alreadyImported: number;
}

export async function scanSSHConfig(): Promise<SSHConfigScan> {
  return (await ConnectionsService.ScanSSHConfig()) as SSHConfigScan;
}

export async function importSSHConfig(
  aliases: string[],
  folder: string,
  answerOnboarding: boolean,
): Promise<SSHConfigImportResult> {
  return (await ConnectionsService.ImportSSHConfig({ aliases, folder, answerOnboarding })) as SSHConfigImportResult;
}

export function answerSSHConfigImport(): Promise<void> {
  return ConnectionsService.AnswerSSHConfigImport();
}
