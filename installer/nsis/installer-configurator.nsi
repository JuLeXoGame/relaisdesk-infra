; =============================================================================
; RelaisDesk Technicien - NSIS Installer Script
; =============================================================================
; Requires NSIS 3.x with MUI2 plugin
; =============================================================================

Unicode true

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

; ---------------------------------------------------------------------------
; Configuration Générale
; ---------------------------------------------------------------------------
!define PRODUCT_NAME "RelaisDesk Technicien"
!define PRODUCT_EXE "configurator.exe"
!ifndef PRODUCT_VERSION
  !define PRODUCT_VERSION "1.0.1"
!endif
!ifndef PAYLOAD_DIR
  !define PAYLOAD_DIR "..\build"
!endif
!define PRODUCT_PUBLISHER "Julien BELLOT EI / RelaisDesk"
!define PRODUCT_WEB_SITE "https://relaisdesk.fr"
!define PRODUCT_DIR_REGKEY "Software\RelaisDesk\Technicien"
!define PRODUCT_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\RelaisDeskTechnicien"
!define PRODUCT_UNINST_ROOT_KEY "HKLM"

; Attributs de l'installeur
Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
!ifndef OUTPUT_FILE
  !define OUTPUT_FILE "RelaisDesk_Technicien_Setup_${PRODUCT_VERSION}.exe"
!endif
OutFile "${OUTPUT_FILE}"
InstallDir "$PROGRAMFILES\RelaisDesk Technicien"
InstallDirRegKey HKLM "${PRODUCT_DIR_REGKEY}" ""
ShowInstDetails show
ShowUnInstDetails show
RequestExecutionLevel admin
SetCompressor /SOLID lzma

; ---------------------------------------------------------------------------
; Configuration MUI
; ---------------------------------------------------------------------------
!define MUI_ABORTWARNING
!define MUI_ICON "..\assets\icon.ico"
!define MUI_UNICON "..\assets\icon.ico"

; Page de bienvenue
!define MUI_WELCOMEPAGE_TITLE "Bienvenue dans l'installation de ${PRODUCT_NAME}"
!define MUI_WELCOMEPAGE_TEXT "Cet assistant va vous guider dans l'installation de ${PRODUCT_NAME} ${PRODUCT_VERSION}.$\r$\n$\r$\nCette application est destinée aux techniciens et dépanneurs informatiques pour gérer leurs licences et générer des codes de téléassistance.$\r$\n$\r$\nCliquez sur Suivant pour continuer."

; Page de fin
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Lancer ${PRODUCT_NAME} maintenant"
!define MUI_FINISHPAGE_LINK "Visiter le site web officiel de RelaisDesk"
!define MUI_FINISHPAGE_LINK_LOCATION "${PRODUCT_WEB_SITE}"

; ---------------------------------------------------------------------------
; Pages d'Installation
; ---------------------------------------------------------------------------
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\assets\license.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; ---------------------------------------------------------------------------
; Pages de Désinstallation
; ---------------------------------------------------------------------------
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; ---------------------------------------------------------------------------
; Langue
; ---------------------------------------------------------------------------
!insertmacro MUI_LANGUAGE "French"

; ---------------------------------------------------------------------------
; Section d'Installation
; ---------------------------------------------------------------------------
Section "Programme principal" SEC01
    SectionIn RO

    SetOutPath "$INSTDIR"
    SetOverwrite on

    ; Fichiers à installer
    File "${PAYLOAD_DIR}\configurator.exe"
    File "..\assets\icon.ico"
    File /oname=RelaisDesk-Licences.txt "..\assets\license.txt"
    File /oname=AGPL-3.0.txt "..\..\rustdesk\LICENCE"

    ; Raccourcis Menu Démarrer
    CreateDirectory "$SMPROGRAMS\RelaisDesk"
    CreateShortcut "$SMPROGRAMS\RelaisDesk\${PRODUCT_NAME}.lnk" "$INSTDIR\${PRODUCT_EXE}" "" "$INSTDIR\icon.ico"
    CreateShortcut "$SMPROGRAMS\RelaisDesk\Désinstaller ${PRODUCT_NAME}.lnk" "$INSTDIR\uninstall.exe"

    ; Raccourci Bureau
    CreateShortcut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\${PRODUCT_EXE}" "" "$INSTDIR\icon.ico"

    ; Enregistrement registre
    WriteRegStr HKLM "${PRODUCT_DIR_REGKEY}" "" "$INSTDIR"

    WriteRegStr HKLM "Software\Classes\relaisdesk" "" "URL:RelaisDesk"
    WriteRegStr HKLM "Software\Classes\relaisdesk" "URL Protocol" ""
    WriteRegStr HKLM "Software\Classes\relaisdesk\shell\open\command" "" '$\"$INSTDIR\${PRODUCT_EXE}$\" --connect-uri $\"%1$\"'

    ; Création du désinstalleur
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; Ajout dans Ajout/Suppression de programmes Windows
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayName" "${PRODUCT_NAME}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "Publisher" "${PRODUCT_PUBLISHER}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "DisplayIcon" "$INSTDIR\icon.ico"
    WriteRegStr ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "UninstallString" "$INSTDIR\uninstall.exe"
    WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoModify" 1
    WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "NoRepair" 1

    ; Calcul taille installée
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

; ---------------------------------------------------------------------------
; Section de Désinstallation
; ---------------------------------------------------------------------------
Section "Uninstall"
    ; Suppression des fichiers
    Delete "$INSTDIR\${PRODUCT_EXE}"
    Delete "$INSTDIR\icon.ico"
    Delete "$INSTDIR\RelaisDesk-Licences.txt"
    Delete "$INSTDIR\AGPL-3.0.txt"
    Delete "$INSTDIR\uninstall.exe"

    ; Suppression des raccourcis
    Delete "$SMPROGRAMS\RelaisDesk\${PRODUCT_NAME}.lnk"
    Delete "$SMPROGRAMS\RelaisDesk\Désinstaller ${PRODUCT_NAME}.lnk"
    RMDir "$SMPROGRAMS\RelaisDesk"
    Delete "$DESKTOP\${PRODUCT_NAME}.lnk"

    ; Suppression du dossier d'installation
    RMDir "$INSTDIR"

    ; Nettoyage registre
    DeleteRegKey ${PRODUCT_UNINST_ROOT_KEY} "${PRODUCT_UNINST_KEY}"
    DeleteRegKey HKLM "${PRODUCT_DIR_REGKEY}"
    ReadRegStr $0 HKLM "Software\Classes\relaisdesk\shell\open\command" ""
    StrCmp $0 '$\"$INSTDIR\${PRODUCT_EXE}$\" --connect-uri $\"%1$\"' 0 +2
    DeleteRegKey HKLM "Software\Classes\relaisdesk"
SectionEnd

; ---------------------------------------------------------------------------
; Fonctions
; ---------------------------------------------------------------------------
Function .onInit
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
