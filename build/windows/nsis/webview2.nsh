!ifndef SSHBROWSE_WEBVIEW2_NSH
!define SSHBROWSE_WEBVIEW2_NSH

!include "WordFunc.nsh"

Var WebView2Available

# Microsoft's documented Evergreen registry keys use pv > 0.0.0.0.
Function SSHBrowse.CheckWebView2
  Push $0
  Push $1
  StrCpy $WebView2Available 0
  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${VersionCompare} "$0" "0.0.0.0" $1
  ${If} $1 == 1
    StrCpy $WebView2Available 1
    Goto checked
  ${EndIf}

  # An all-users installation needs a machine runtime, not just the admin's.
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${VersionCompare} "$0" "0.0.0.0" $1
    ${If} $1 == 1
      StrCpy $WebView2Available 1
    ${EndIf}
  !endif

checked:
  Pop $1
  Pop $0
FunctionEnd

Function SSHBrowse.EnsureWebView2
  Call SSHBrowse.CheckWebView2
  ${If} $WebView2Available == 1
    Return
  ${EndIf}

  DetailPrint "Installing Microsoft Edge WebView2 Runtime..."
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\webview2bootstrapper"
  File "MicrosoftEdgeWebview2Setup.exe"
  ClearErrors
  ExecWait '"$PLUGINSDIR\webview2bootstrapper\MicrosoftEdgeWebview2Setup.exe" /silent /install' $0
  ${If} ${Errors}
    MessageBox MB_OK|MB_ICONSTOP "Microsoft Edge WebView2 Runtime setup could not start. Run the SSHBrowse installer again."
    SetErrorLevel 1
    Abort
  ${EndIf}

  # Verify availability even after a successful exit. A usable runtime also
  # permits continuation after a nonzero status such as reboot required.
  Call SSHBrowse.CheckWebView2
  ${If} $WebView2Available != 1
    MessageBox MB_OK|MB_ICONSTOP "Microsoft Edge WebView2 Runtime could not be installed (setup exit code: $0). Check your Internet connection and run the SSHBrowse installer again."
    SetErrorLevel 1
    Abort
  ${EndIf}
FunctionEnd

!endif
