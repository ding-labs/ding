#ifndef DingVersion
  #error Supply /DDingVersion and /DPayload
#endif
#ifndef DingArch
  #define DingArch "amd64"
#endif
[Setup]
AppId=ing.ding.watch
AppName=Ding
AppVersion={#DingVersion}
AppPublisher=Ding Labs
AppPublisherURL=https://ding.ing
DefaultDirName={localappdata}\Programs\Ding
DisableDirPage=yes
PrivilegesRequired=lowest
UsePreviousPrivileges=no
#if DingArch == "arm64"
ArchitecturesAllowed=arm64
ArchitecturesInstallIn64BitMode=arm64
#else
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
#endif
MinVersion=10.0.17763
CloseApplications=no
RestartApplications=no
OutputBaseFilename=ding_windows_{#DingArch}_setup
Compression=lzma2
SolidCompression=yes
UninstallDisplayIcon={app}\ding.exe
#ifdef ReleaseSigning
SignTool=ding
SignedUninstaller=yes
#endif

[Files]
Source: "{#Payload}\ding.exe"; DestDir: "{app}"
Source: "{#Payload}\installation-owner"; DestDir: "{app}"
Source: "{#Payload}\LICENSE"; DestDir: "{app}"

[Icons]
Name: "{userprograms}\Ding Console"; Filename: "{app}\ding.exe"; Parameters: "ui"; AppUserModelID: "Ding"
Name: "{userprograms}\Ding setup"; Filename: "{app}\ding.exe"; Parameters: "setup"

[Registry]
Root: HKCU; Subkey: "Software\Classes\ding-watch"; ValueType: string; ValueData: "URL:Ding watch notification"
Root: HKCU; Subkey: "Software\Classes\ding-watch"; ValueType: string; ValueName: "URL Protocol"; ValueData: ""
Root: HKCU; Subkey: "Software\Classes\ding-watch\shell\open\command"; ValueType: string; ValueData: """{app}\ding.exe"" notification-open -- ""%1"""

[Run]
Filename: "{app}\ding.exe"; Parameters: "setup --yes"; Description: "Start Ding at sign-in and open the Console (no account required)"; Flags: postinstall nowait skipifsilent unchecked

[Code]
function ProtocolCommand(): String;
begin
  Result := '"' + ExpandConstant('{app}\ding.exe') + '" notification-open -- "%1"';
end;

function ProtocolConflict(): Boolean;
var Existing: String;
begin
  Result := RegKeyExists(HKCU, 'Software\Classes\ding-watch') and
    (not RegQueryStringValue(HKCU, 'Software\Classes\ding-watch\shell\open\command', '', Existing) or
     (Existing <> ProtocolCommand()));
end;

function HasManagedState(): Boolean;
begin
  Result := FileExists(ExpandConstant('{userappdata}\ding\watch\installation.json'));
end;

function RunOwnedService(Action: String): Boolean;
var ExitCode: Integer;
begin
  Result := Exec(ExpandConstant('{app}\ding.exe'), 'service ' + Action, '', SW_HIDE, ewWaitUntilTerminated, ExitCode) and (ExitCode = 0);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  if ProtocolConflict() then begin
    Result := 'Another installation owns the ding-watch notification protocol. Resolve that installation before continuing.';
    Exit;
  end;
  if FileExists(ExpandConstant('{app}\ding.exe')) and HasManagedState() then
    if not RunOwnedService('stop') then
      Result := 'Ding could not safely stop its owned service. Run ding status and repair it before upgrading. State has been retained.';
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var Existing: String;
begin
  if CurUninstallStep = usPostUninstall then
    if RegQueryStringValue(HKCU, 'Software\Classes\ding-watch\shell\open\command', '', Existing) and
       (Existing = ProtocolCommand()) then
      RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\ding-watch');
end;

function InitializeUninstall(): Boolean;
begin
  Result := True;
  if HasManagedState() then
    Result := RunOwnedService('uninstall');
  if not Result then
    MsgBox('Ding could not remove its owned startup task. Inspect ding status before uninstalling. Your data is retained.', mbError, MB_OK);
end;

// No deletion rule targets AppData\Roaming\ding or any custom state directory.
