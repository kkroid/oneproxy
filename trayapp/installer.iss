; OneProxy Installer Script — Inno Setup 6
#define AppName "OneProxy"
#define AppVersion "0.5.0"
#define AppPublisher "OneProxy Contributors"
#define AppURL "https://github.com/kkroid/oneproxy"
#define AppExeName "oneproxy-tray.exe"

[Setup]
AppId={{7A3F8C9E-1D2B-4A56-B789-012345ABCDEF}
AppName={#AppName}
AppVersion={#AppVersion}
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
PrivilegesRequired=admin

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
; Main executable and core DLL
Source: "oneproxy-tray.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "oneproxy.dll"; DestDir: "{app}"; Flags: ignoreversion

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
Source: "config.json"; DestDir: "{userprofile}\.oneproxy"; Flags: ignoreversion onlyifdoesntexist

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
