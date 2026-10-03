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

Function SSHBrowse.RequireWebView2
  Call SSHBrowse.CheckWebView2
  ${If} $WebView2Available == 1
    Return
  ${EndIf}

  MessageBox MB_OK|MB_ICONSTOP "SSHBrowse requires Microsoft Edge WebView2 Runtime, which is normally included with Windows 11. Install or repair WebView2 Runtime and run SSHBrowse Setup again."
  SetErrorLevel 1
  Abort
FunctionEnd

!endif
