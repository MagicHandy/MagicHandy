#ifndef SourceDir
  #error SourceDir must point at the prepared MagicHandy release payload.
#endif
#ifndef OutputDir
  #define OutputDir "."
#endif
#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
#endif
#ifndef ArtifactVersion
  #define ArtifactVersion "0.0.0-dev"
#endif
#ifndef NumericVersion
  #define NumericVersion "0.0.0.0"
#endif
#ifndef InstallerSetupArchitecture
  #define InstallerSetupArchitecture "x64"
#endif
#ifndef InstallerCompression
  #define InstallerCompression "zip/9"
#endif
#ifndef InstallerSolidCompression
  #define InstallerSolidCompression "no"
#endif

[Setup]
AppId={{A9859C5A-AD69-4D2E-91DA-809D109984DA}
AppName=MagicHandy
AppVersion={#AppVersion}
AppVerName=MagicHandy {#AppVersion}
AppPublisher=MagicHandy
AppPublisherURL=https://github.com/MagicHandy/MagicHandy
AppSupportURL=https://github.com/MagicHandy/MagicHandy/issues
AppUpdatesURL=https://github.com/MagicHandy/MagicHandy/releases
LicenseFile={#SourceDir}\LICENSE
DefaultDirName={autopf}\MagicHandy
DisableDirPage=no
UsePreviousAppDir=yes
DefaultGroupName=MagicHandy
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename=MagicHandy-{#ArtifactVersion}-windows-amd64-setup
SetupArchitecture={#InstallerSetupArchitecture}
Compression={#InstallerCompression}
SolidCompression={#InstallerSolidCompression}
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog
UsePreviousPrivileges=yes
UsePreviousTasks=yes
WizardStyle=modern
SetupLogging=yes
CloseApplications=yes
CloseApplicationsFilter=magichandy.exe
RestartApplications=no
UninstallDisplayIcon={app}\magichandy.exe
UninstallDisplayName=MagicHandy
VersionInfoDescription=MagicHandy Windows installer
VersionInfoProductName=MagicHandy
VersionInfoProductVersion={#NumericVersion}
VersionInfoVersion={#NumericVersion}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional shortcuts:"; Flags: unchecked

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\MagicHandy"; Filename: "{app}\magichandy.exe"; Parameters: "-open-browser"; WorkingDir: "{app}"
Name: "{autodesktop}\MagicHandy"; Filename: "{app}\magichandy.exe"; Parameters: "-open-browser"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\magichandy.exe"; Parameters: "-open-browser"; Description: "Open MagicHandy"; WorkingDir: "{app}"; Flags: postinstall nowait skipifsilent runasoriginaluser

[UninstallRun]
Filename: "{app}\magichandy.exe"; Parameters: "-prepare-uninstall"; WorkingDir: "{app}"; Flags: runhidden skipifdoesntexist; RunOnceId: "PrepareUninstall"

[Code]
var
  PurgeUserData: Boolean;
  UserDataRemovalFailed: Boolean;

function InitializeSetup(): Boolean;
begin
  Result := True;
end;

function HasUninstallSwitch(const SwitchName: String): Boolean;
var
  Index: Integer;
begin
  Result := False;
  for Index := 1 to ParamCount do
  begin
    if CompareText(ParamStr(Index), SwitchName) = 0 then
    begin
      Result := True;
      Exit;
    end;
  end;
end;

function MagicHandyUserDataPath(): String;
begin
  Result := ExpandConstant('{userappdata}\MagicHandy');
end;

function InitializeUninstall(): Boolean;
var
  KeepRequested: Boolean;
  PurgeRequested: Boolean;
  Answer: Integer;
begin
  Result := True;
  PurgeUserData := False;
  UserDataRemovalFailed := False;
  KeepRequested := HasUninstallSwitch('/KEEPUSERDATA');
  PurgeRequested := HasUninstallSwitch('/PURGEUSERDATA');

  if KeepRequested and PurgeRequested then
  begin
    Log('Uninstall aborted: /KEEPUSERDATA and /PURGEUSERDATA cannot be combined.');
    if not UninstallSilent then
      SuppressibleMsgBox(
        'Choose either /KEEPUSERDATA or /PURGEUSERDATA, not both.',
        mbError,
        MB_OK,
        IDOK
      );
    Result := False;
    Exit;
  end;

  if KeepRequested then
  begin
    Log('Uninstall will preserve app-owned user data by explicit request.');
    Exit;
  end;

  if PurgeRequested or UninstallSilent then
  begin
    PurgeUserData := True;
    Log('Uninstall will remove app-owned user data.');
    Exit;
  end;

  Answer := SuppressibleMsgBox(
    'Also remove MagicHandy app data?' + #13#10 + #13#10 +
    'Yes (recommended for a clean reinstall) deletes settings, chat history, personas, logs, imported models, managed runtimes, and voice modules from:' + #13#10 +
    MagicHandyUserDataPath() + #13#10 + #13#10 +
    'No keeps that data for a later reinstall. External Ollama models, media and funscript folders, and source checkouts are never removed.',
    mbConfirmation,
    MB_YESNOCANCEL or MB_DEFBUTTON1,
    IDYES
  );
  if Answer = IDCANCEL then
  begin
    Result := False;
    Exit;
  end;
  PurgeUserData := Answer = IDYES;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  UserDataPath: String;
begin
  if CurUninstallStep <> usPostUninstall then
    Exit;

  UserDataPath := MagicHandyUserDataPath();
  if PurgeUserData and DirExists(UserDataPath) then
  begin
    UserDataRemovalFailed := not DelTree(UserDataPath, True, True, True);
    if UserDataRemovalFailed then
      Log('Could not completely remove app-owned user data: ' + UserDataPath)
    else
      Log('Removed app-owned user data: ' + UserDataPath);
  end;

  if UninstallSilent then
    Exit;

  if UserDataRemovalFailed then
    SuppressibleMsgBox(
      'MagicHandy program files were removed, but some app data could not be deleted. Close any remaining MagicHandy worker processes and remove this folder before reinstalling:' + #13#10 +
      UserDataPath,
      mbError,
      MB_OK,
      IDOK
    )
  else if PurgeUserData then
    SuppressibleMsgBox(
      'MagicHandy was removed, including its app-owned settings, history, models, runtimes, and voice modules. External Ollama models, media folders, and source checkouts were unchanged.',
      mbInformation,
      MB_OK,
      IDOK
    )
  else
    SuppressibleMsgBox(
      'MagicHandy was removed. App-owned settings, history, models, runtimes, and voice modules remain at:' + #13#10 +
      UserDataPath,
      mbInformation,
      MB_OK,
      IDOK
    );
end;
