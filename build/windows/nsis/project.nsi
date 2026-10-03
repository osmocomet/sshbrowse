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

!insertmacro MUI_PAGE_WELCOME
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
  ${IfNot} ${AtLeastWin11}
  ${OrIfNot} ${IsNativeAMD64}
    MessageBox MB_OK|MB_ICONSTOP "SSHBrowse supports Windows 11 x64 only."
    SetErrorLevel 64
    Abort
  ${EndIf}
  Call SSHBrowse.RequireWebView2
  Return
silentInstall:
  SetErrorLevel 65
  Abort
FunctionEnd

Section
  !insertmacro wails.setShellContext
  SetOutPath $INSTDIR
  !insertmacro wails.files
  File /oname=SSHBrowse-LICENSE.txt "..\..\..\LICENSE"
  File /oname=SSHBrowse-THIRD-PARTY-NOTICES.txt "..\..\..\docs\legal\THIRD_PARTY_NOTICES.txt"
  CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
  !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
  !insertmacro wails.setShellContext
  RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
  Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
  Delete "$INSTDIR\SSHBrowse-LICENSE.txt"
  Delete "$INSTDIR\SSHBrowse-NOTICE.txt"
  Delete "$INSTDIR\SSHBrowse-THIRD-PARTY-NOTICES.txt"
  Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
  Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
  !insertmacro wails.deleteUninstaller
  RMDir "$INSTDIR"
SectionEnd
