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
DefaultGroupName=MagicHandy
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename=MagicHandy-{#ArtifactVersion}-windows-amd64-setup
Compression=lzma2/ultra64
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=dialog
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
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts"; Flags: unchecked

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\MagicHandy"; Filename: "{app}\magichandy.exe"; Parameters: "-open-browser"; WorkingDir: "{app}"
Name: "{autodesktop}\MagicHandy"; Filename: "{app}\magichandy.exe"; Parameters: "-open-browser"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\magichandy.exe"; Parameters: "-setup -open-browser"; Description: "Open MagicHandy setup"; WorkingDir: "{app}"; Flags: postinstall nowait skipifsilent runasoriginaluser

[Code]
function InitializeSetup(): Boolean;
begin
  Result := True;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usPostUninstall) and (not UninstallSilent) then
    MsgBox(
      'MagicHandy was removed. Your settings, models, voice modules, and history remain at ' +
      ExpandConstant('{userappdata}\MagicHandy') + ' and were not deleted.',
      mbInformation,
      MB_OK
    );
end;
