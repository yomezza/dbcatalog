#define AppVersion "0.5.0"
[Setup]
AppId={{77CC22CF-5766-4A80-A049-853E7A9B22E0}
AppName=CatalogDB
AppVersion={#AppVersion}
DefaultDirName={autopf}\CatalogDB
DefaultGroupName=CatalogDB
OutputDir=..\..\build\bin
OutputBaseFilename=CatalogDB_Setup_{#AppVersion}
UninstallDisplayIcon={app}\CatalogDB.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
MinVersion=10.0.19041
CloseApplications=yes

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[Tasks]
Name: "desktopicon"; Description: "Создать ярлык на рабочем столе"; GroupDescription: "Ярлыки:"

[Files]
Source: "..\..\build\bin\CatalogDB.exe"; DestDir: "{app}"; Flags: ignoreversion

Source: "..\vendor\MicrosoftEdgeWebView2Setup.exe"; DestDir: "{tmp}"; Flags: dontcopy

[Icons]
Name: "{group}\CatalogDB"; Filename: "{app}\CatalogDB.exe"
Name: "{autodesktop}\CatalogDB"; Filename: "{app}\CatalogDB.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\CatalogDB.exe"; Description: "Запустить CatalogDB"; Flags: nowait postinstall skipifsilent runasoriginaluser

[Code]
const
  RuntimeKey = 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
function ValidRuntimeVersion(Root: Integer): Boolean;
var Version: String;
begin
  Result := RegQueryStringValue(Root, RuntimeKey, 'pv', Version) and (Version <> '') and (Version <> '0.0.0.0');
end;
function HasWebView2(): Boolean;
begin
  Result := ValidRuntimeVersion(HKLM32) or ValidRuntimeVersion(HKCU32);
end;
function PrepareToInstall(var NeedsRestart: Boolean): String;
var ExitCode: Integer;
begin
  Result := '';
  if not HasWebView2() then begin
    ExtractTemporaryFile('MicrosoftEdgeWebView2Setup.exe');
    if not Exec(ExpandConstant('{tmp}\MicrosoftEdgeWebView2Setup.exe'), '/silent /install', '', SW_HIDE, ewWaitUntilTerminated, ExitCode) then
      Result := 'Не удалось запустить установщик Microsoft WebView2.'
    else if not HasWebView2() then
      Result := 'WebView2 не установлен. Проверьте доступ к серверам Microsoft или используйте корпоративный офлайн-установщик WebView2 и повторите установку.';
  end;
end;
