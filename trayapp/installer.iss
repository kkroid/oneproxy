; OneProxy Installer Script — Inno Setup 6
#define AppName "OneProxy"
#define AppVersion "0.8.3"
#define AppPublisher "OneProxy Contributors"
#define AppURL "https://github.com/kkroid/oneproxy"
#define AppExeName "oneproxy-tray.exe"

[Setup]
AppId={{7A3F8C9E-1D2B-4A56-B789-012345ABCDEF}
AppName={#AppName}
AppVersion={#AppVersion}
VersionInfoVersion=0.8.3.0
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
AllowNoIcons=yes
OutputDir=..\..\dist
OutputBaseFilename=OneProxy-{#AppVersion}-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible
ArchitecturesAllowed=x64compatible
MinVersion=10.0.10240
PrivilegesRequired=admin
; Our directory-scoped shutdown owns this lifecycle; do not let Restart Manager
; close unrelated applications or schedule replacement of running DLLs on reboot.
CloseApplications=no
RestartApplications=no
AllowCancelDuringInstall=no
; Enabled only for explicit signed builds via ISCC /DSignToolCommand.
; The command is supplied as /Soneproxy-sign=<command>.
#ifdef SignToolCommand
SignTool=oneproxy-sign
SignedUninstaller=yes
#endif

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "oneproxy-upgrade.dll"; Flags: dontcopy
; Main executable and core DLL
Source: "oneproxy-tray.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "oneproxy.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "oneproxy.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "dxcompiler.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "dxil.dll"; DestDir: "{app}"; Flags: ignoreversion

; App-local MSVC release runtime from the compiler's x64 redistributable directory.
; No system-wide runtime installation or network download is needed.
Source: "vcruntime140*.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "msvcp140*.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "concrt140.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "vccorlib140.dll"; DestDir: "{app}"; Flags: ignoreversion

; Qt6 runtime DLLs (only the ones windeployqt deploys)
Source: "Qt6Core.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "Qt6Gui.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "Qt6Widgets.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "Qt6Network.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "Qt6Svg.dll"; DestDir: "{app}"; Flags: ignoreversion

; Qt6 plugins (empty dirs will be skipped by Inno)
Source: "platforms\*"; DestDir: "{app}\platforms"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "styles\*"; DestDir: "{app}\styles"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "imageformats\*"; DestDir: "{app}\imageformats"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "iconengines\*"; DestDir: "{app}\iconengines"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "tls\*"; DestDir: "{app}\tls"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "generic\*"; DestDir: "{app}\generic"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "networkinformation\*"; DestDir: "{app}\networkinformation"; Flags: ignoreversion recursesubdirs createallsubdirs

; Icons
Source: "green.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "yellow.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "red.ico"; DestDir: "{app}"; Flags: ignoreversion

; sing-box proxy core
Source: "bin\*"; DestDir: "{app}\bin"; Flags: ignoreversion recursesubdirs createallsubdirs

; Default config — write to user profile, not Program Files (writable without admin)
; Source is config-placeholder.json (example config) to avoid accidentally
; packaging a real config.json from the build directory.
Source: "config-placeholder.json"; DestDir: "{%USERPROFILE}\.oneproxy"; DestName: "config.json"; Flags: ignoreversion onlyifdoesntexist

; Also keep a copy in app dir as fallback template — DLL uses it to recover if user deletes config.json
Source: "config-placeholder.json"; DestDir: "{app}"; Flags: ignoreversion

; Create empty logs dir
[Dirs]
Name: "{app}\logs"

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"
Name: "{group}\Uninstall {#AppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional icons:"

[Run]
Filename: "{app}\{#AppExeName}"; Description: "Launch {#AppName}"; Flags: nowait postinstall skipifsilent

[Code]
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Command, Executable: String;
begin
  if CurUninstallStep <> usUninstall then
    Exit;
  { Only this user's exact installation is ours. Never delete the Run key,
    other values, other users' entries, or a different OneProxy installation. }
  if RegQueryStringValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run',
      'OneProxy', Command) then
  begin
    Executable := ExpandConstant('{app}\{#AppExeName}');
    StringChangeEx(Command, '/', '\', True);
    if (CompareText(Command, Executable) = 0) or
       (CompareText(Command, '"' + Executable + '"') = 0) then
      if not RegDeleteValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run', 'OneProxy') then
        Log('Could not remove the OneProxy startup entry.');
  end;
end;

function PrepareUpgrade(Directory: String): Integer;
  external 'PrepareUpgrade@files:oneproxy-upgrade.dll stdcall';
procedure FinishUpgrade();
  external 'FinishUpgrade@files:oneproxy-upgrade.dll stdcall';
procedure ReleaseUninstallGuards();
  external 'ReleaseUninstallGuards@files:oneproxy-upgrade.dll stdcall';
procedure UpgradeError(Buffer: String; Count: Integer);
  external 'UpgradeError@files:oneproxy-upgrade.dll stdcall';

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Detail: String;
begin
  Result := '';
  if not ForceDirectories(ExpandConstant('{app}')) then
  begin
    Result := 'Cannot create the installation directory. Check permissions and retry, or cancel Setup.';
    Exit;
  end;
  while PrepareUpgrade(ExpandConstant('{app}')) = 0 do
  begin
    SetLength(Detail, 2048);
    UpgradeError(Detail, 2048);
    Detail := Copy(Detail, 1, Pos(#0, Detail) - 1);
    Log(Detail);
    Result := 'OneProxy could not be prepared for upgrade safely. ' + Detail + #13#10 +
      'No application files have been replaced. Resolve the reported problem and click Retry, or cancel Setup.';
    if SuppressibleMsgBox(Result, mbError, MB_RETRYCANCEL, IDCANCEL) <> IDRETRY then
      Exit;
  end;
  Result := '';
end;

function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo,
  MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
begin
  Result := MemoDirInfo + NewLine + NewLine +
    'Setup will close OneProxy in this installation directory. Your proxy connection will be briefly interrupted. Your configuration and subscriptions will be kept.';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  { Inno reads/appends the DAT and replaces the EXE during installation, before
    ssPostInstall. Hand back only its files; keep all application guards. }
  if CurStep = ssInstall then ReleaseUninstallGuards();
  { Release before the optional post-install launch. }
  if CurStep = ssPostInstall then FinishUpgrade();
end;

procedure DeinitializeSetup();
begin
  FinishUpgrade();
end;
