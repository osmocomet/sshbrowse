Unicode true

!include "wails_tools.nsh"
!include "webview2.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName" "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright" "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName" "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
!define MUI_LICENSEPAGE_CHECKBOX
!define MUI_LICENSEPAGE_CHECKBOX_TEXT "I accept Microsoft's terms for Microsoft Edge WebView2 Runtime"
!define MUI_LICENSEPAGE_TEXT_TOP "These Microsoft terms apply only to Microsoft Edge WebView2 Runtime. SSHBrowse has separate terms in SSHBrowse-NOTICE.txt."
!define MUI_LICENSEPAGE_TEXT_BOTTOM "Select the checkbox to accept these terms for WebView2 Runtime and continue installing SSHBrowse."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\..\docs\legal\MICROSOFT_WEBVIEW2_LICENSE.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
!if "${WAILS_INSTALL_SCOPE}" == "user"
  InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show

Function .onInit
  IfSilent silentInstall
  !insertmacro wails.checkArchitecture
  Return
silentInstall:
  SetErrorLevel 65
  Abort
FunctionEnd

Section
  !insertmacro wails.setShellContext
  Call SSHBrowse.EnsureWebView2
  SetOutPath $INSTDIR
  !insertmacro wails.files
  File /oname=SSHBrowse-NOTICE.txt "..\..\..\NOTICE"
  File /oname=SSHBrowse-THIRD-PARTY-NOTICES.txt "..\..\..\docs\legal\THIRD_PARTY_NOTICES.txt"
  File /oname=Microsoft-Edge-WebView2-Runtime-License.txt "..\..\..\docs\legal\MICROSOFT_WEBVIEW2_LICENSE.txt"
  CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
  !insertmacro wails.setShellContext
  RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
  Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
  Delete "$INSTDIR\SSHBrowse-NOTICE.txt"
  Delete "$INSTDIR\SSHBrowse-THIRD-PARTY-NOTICES.txt"
  Delete "$INSTDIR\Microsoft-Edge-WebView2-Runtime-License.txt"
  Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
  Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
  !insertmacro wails.deleteUninstaller
  RMDir "$INSTDIR"
SectionEnd
