; =============================================================================
; RelaisDesk - NSIS Installer Script
; =============================================================================
; Requires NSIS 3.x with MUI2 plugin
; =============================================================================

Unicode true

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

; ---------------------------------------------------------------------------
; General Configuration
; ---------------------------------------------------------------------------
!define PRODUCT_NAME "RelaisDesk"
!ifndef PRODUCT_VERSION
  !define PRODUCT_VERSION "1.0.1"
!endif
!ifndef PAYLOAD_DIR
  !define PAYLOAD_DIR "..\build"
!endif
!define PRODUCT_PUBLISHER "Julien BELLOT EI / RelaisDesk"
!define PRODUCT_WEB_SITE "https://relaisdesk.fr"
!define PRODUCT_DIR_REGKEY "Software\${PRODUCT_NAME}"
!define PRODUCT_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"
!define PRODUCT_UNINST_ROOT_KEY "HKLM"

; Installer attributes
Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
!ifndef OUTPUT_FILE
  !define OUTPUT_FILE "RelaisDesk_Setup.exe"
!endif
OutFile "${OUTPUT_FILE}"
InstallDir "$PROGRAMFILES\${PRODUCT_NAME}"
InstallDirRegKey HKLM "${PRODUCT_DIR_REGKEY}" ""
ShowInstDetails show
ShowUnInstDetails show
RequestExecutionLevel admin
SetCompressor /SOLID lzma

; ---------------------------------------------------------------------------
; MUI Configuration
; ---------------------------------------------------------------------------
!define MUI_ABORTWARNING
!define MUI_ICON "..\assets\icon.ico"
!define MUI_UNICON "..\assets\icon.ico"

; Welcome page configuration
!define MUI_WELCOMEPAGE_TITLE "Bienvenue dans l'assistant d'installation de ${PRODUCT_NAME}"
!define MUI_WELCOMEPAGE_TEXT "Cet assistant va vous guider dans l'installation de ${PRODUCT_NAME} ${PRODUCT_VERSION}.$\r$\n$\r$\n${PRODUCT_NAME} vous permet de bénéficier d'un accès distant sécurisé et performant.$\r$\n$\r$\nCliquez sur Suivant pour continuer."

; Finish page configuration
!define MUI_FINISHPAGE_RUN "$INSTDIR\configurator.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Lancer ${PRODUCT_NAME} Technicien"
!define MUI_FINISHPAGE_LINK "Visiter le site web officiel de ${PRODUCT_NAME}"
!define MUI_FINISHPAGE_LINK_LOCATION "${PRODUCT_WEB_SITE}"

; ---------------------------------------------------------------------------
; Installer Pages (MUST BE DECLARED BEFORE MUI_LANGUAGE)
; ---------------------------------------------------------------------------
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\assets\license.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; ---------------------------------------------------------------------------
; Uninstaller Pages (MUST BE DECLARED BEFORE MUI_LANGUAGE)
; ---------------------------------------------------------------------------
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; ---------------------------------------------------------------------------
; Language Definition (After Pages)
; ---------------------------------------------------------------------------
!insertmacro MUI_LANGUAGE "French"

; ---------------------------------------------------------------------------
; Installer Section
; ---------------------------------------------------------------------------
Section "Programme principal" SEC01
    SectionIn RO

    ; Set output path to installation directory
    SetOutPath "$INSTDIR"
    SetOverwrite on

    ; Install files
    File "${PAYLOAD_DIR}\configurator.exe"
    File "${PAYLOAD_DIR}\viewer.exe"
    File "..\assets\icon.ico"
    File /oname=RelaisDesk-Licences.txt "..\assets\license.txt"
    File /oname=AGPL-3.0.txt "..\..\rustdesk\LICENCE"

    ; Create Start Menu shortcuts
    CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME} Technicien.lnk" "$INSTDIR\configurator.exe" "" "$INSTDIR\icon.ico"
    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME} Client Viewer.lnk" "$INSTDIR\viewer.exe" "" "$INSTDIR\icon.ico"
    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\Désinstaller ${PRODUCT_NAME}.lnk" "$INSTDIR\uninstall.exe"

    ; Create Desktop shortcuts
    CreateShortcut "$DESKTOP\${PRODUCT_NAME} Technicien.lnk" "$INSTDIR\configurator.exe" "" "$INSTDIR\icon.ico"

    ; Store installation directory in registry
    WriteRegStr HKLM "${PRODUCT_DIR_REGKEY}" "" "$INSTDIR"
    WriteRegStr HKLM "Software\Classes\relaisdesk" "" "URL:RelaisDesk"
    WriteRegStr HKLM "Software\Classes\relaisdesk" "URL Protocol" ""
    WriteRegStr HKLM "Software\Classes\relaisdesk\shell\open\command" "" '$\"$INSTDIR\configurator.exe$\" --connect-uri $\"%1$\"'

    ; Create uninstaller
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; Register in Add/Remove Programs
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayName" "${PRODUCT_NAME}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "Publisher" "${PRODUCT_PUBLISHER}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayIcon" "$INSTDIR\icon.ico"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "UninstallString" "$INSTDIR\uninstall.exe"
    WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoModify" 1
    WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoRepair" 1

    ; Calculate and write installed size
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "EstimatedSize" "$0"

    ; Enrôlement de parc optionnel (déploiement de masse) :
    ;   RelaisDesk_Setup.exe /S /ENROLLCODE=PARK-XXXX /PASSWORD=secret
    ; Le mot de passe transite par variable d'environnement (jamais en argv
    ; du viewer) et la variable est nettoyée juste après usage. En cas
    ; d'échec, l'installation est marquée en erreur pour que l'outil de
    ; déploiement (GPO/Intune/SCCM) la rejoue.
    StrCpy $R0 ""
    ${GetOptions} $CMDLINE "/ENROLLCODE=" $R0
    ${If} $R0 != ""
        DetailPrint "Enrôlement du poste..."
        StrCpy $R1 ""
        ${GetOptions} $CMDLINE "/PASSWORD=" $R1
        ${If} $R1 != ""
            System::Call 'kernel32::SetEnvironmentVariable(t "RELAISDESK_ENROLL_PASSWORD", t "$R1")'
        ${EndIf}
        ExecWait '"$INSTDIR\viewer.exe" --enroll $R0' $R2
        System::Call 'kernel32::SetEnvironmentVariable(t "RELAISDESK_ENROLL_PASSWORD", t "")'
        ${If} $R2 != 0
            DetailPrint "Échec de l'enrôlement (code $R2)."
            IfSilent +2 0
            MessageBox MB_ICONEXCLAMATION "Installation terminée mais l'enrôlement a échoué (code $R2). Relancez avec un code valide."
            SetErrorLevel 1
            Abort "Échec de l'enrôlement du poste."
        ${EndIf}
        DetailPrint "Poste enrôlé."
    ${EndIf}
SectionEnd

; ---------------------------------------------------------------------------
; Uninstaller Section
; ---------------------------------------------------------------------------
Section "Uninstall"
    ExecWait '$\"$INSTDIR\viewer.exe$\" --unenroll' $0
    StrCmp $0 0 +3
    MessageBox MB_ICONSTOP "Impossible d'arrêter le service de parc. Désinstallation annulée."
    Abort
    ; Remove files
    Delete "$INSTDIR\configurator.exe"
    Delete "$INSTDIR\viewer.exe"
    Delete "$INSTDIR\icon.ico"
    Delete "$INSTDIR\RelaisDesk-Licences.txt"
    Delete "$INSTDIR\AGPL-3.0.txt"
    Delete "$INSTDIR\uninstall.exe"

    ; Remove shortcuts
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME} Technicien.lnk"
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME} Client Viewer.lnk"
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\Désinstaller ${PRODUCT_NAME}.lnk"
    RMDir "$SMPROGRAMS\${PRODUCT_NAME}"
    Delete "$DESKTOP\${PRODUCT_NAME} Technicien.lnk"

    ; Remove installation directory (only if empty)
    RMDir "$INSTDIR"

    ; Remove registry entries
    DeleteRegKey ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}"
    DeleteRegKey HKLM "${PRODUCT_DIR_REGKEY}"
    ReadRegStr $0 HKLM "Software\Classes\relaisdesk\shell\open\command" ""
    StrCmp $0 '$\"$INSTDIR\configurator.exe$\" --connect-uri $\"%1$\"' 0 +2
    DeleteRegKey HKLM "Software\Classes\relaisdesk"
SectionEnd

; ---------------------------------------------------------------------------
; Callback Functions
; ---------------------------------------------------------------------------
Function .onInit
    ; Check for admin rights
    UserInfo::GetAccountType
    Pop $0
    ${If} $0 != "admin"
        MessageBox MB_ICONSTOP "Droits administrateur requis pour l'installation."
        Abort
    ${EndIf}
FunctionEnd

Function un.onInit
    MessageBox MB_ICONQUESTION|MB_YESNO "Voulez-vous vraiment désinstaller ${PRODUCT_NAME} ?" IDYES +2
    Abort
FunctionEnd
