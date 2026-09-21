!include "MUI2.nsh"

Name "RelaisDesk Viewer"
OutFile "RelaisDesk-Viewer-Setup.exe"
InstallDir "$PROGRAMFILES\RelaisDeskViewer"
RequestExecutionLevel admin

!define MUI_ABORTWARNING
!define MUI_ICON "..\assets\icon.ico"
!define MUI_UNICON "..\assets\icon.ico"
!define MUI_LANGDLL_REGISTRY_ROOT "HKCU"
!define MUI_LANGDLL_REGISTRY_KEY "Software\RelaisDeskViewer"
!define MUI_LANGDLL_REGISTRY_VALUENAME "Installer Language"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "French"

Section "Installation" SecDummy
    SetOutPath "$INSTDIR"
    File "..\viewer\viewer.exe"
    File "..\assets\icon.ico"

    WriteUninstaller "$INSTDIR\Uninstall.exe"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RelaisDeskViewer" "DisplayName" "RelaisDesk Viewer"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RelaisDeskViewer" "UninstallString" "$INSTDIR\Uninstall.exe"
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RelaisDeskViewer" "DisplayIcon" "$INSTDIR\icon.ico"

    CreateShortcut "$DESKTOP\RelaisDesk Viewer.lnk" "$INSTDIR\viewer.exe" "" "$INSTDIR\icon.ico"
    CreateShortcut "$SMPROGRAMS\RelaisDesk Viewer.lnk" "$INSTDIR\viewer.exe" "" "$INSTDIR\icon.ico"

    ExecShell "open" "$INSTDIR\viewer.exe"
SectionEnd

Section "Uninstall"
    ExecWait '$\"$INSTDIR\viewer.exe$\" --unenroll' $0
    StrCmp $0 0 +3
    MessageBox MB_ICONSTOP "Impossible d'arrêter le service de parc. Désinstallation annulée."
    Abort
    Delete "$INSTDIR\viewer.exe"
    Delete "$INSTDIR\Uninstall.exe"
    Delete "$DESKTOP\RelaisDesk Viewer.lnk"
    Delete "$SMPROGRAMS\RelaisDesk Viewer.lnk"

    RMDir "$INSTDIR"
    DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RelaisDeskViewer"
SectionEnd
