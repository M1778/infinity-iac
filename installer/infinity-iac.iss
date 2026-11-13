#define AppName "Infinity-IaC"
#define AppPublisher "M1778"
#define AppURL "https://github.com/M1778/infinity-iac"

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceBinary
  #define SourceBinary "..\dist\infinity-iac_windows_amd64.exe"
#endif

[Setup]
AppId={{D6D3372A-36D4-4B8D-AD78-5E4A2DC3D4B9}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}/releases
DefaultDirName={localappdata}\Programs\Infinity-IaC
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesInstallIn64BitMode=x64compatible
ChangesEnvironment=yes
OutputDir=..\dist
OutputBaseFilename=infinity-iac-setup
Compression=lzma
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\infinity-iac.exe

[Files]
Source: "{#SourceBinary}"; DestDir: "{app}"; DestName: "infinity-iac.exe"; Flags: ignoreversion

[Icons]
Name: "{group}\Infinity-IaC Command Prompt"; Filename: "{cmd}"; Parameters: "/K ""{app}\infinity-iac.exe"" --help"; WorkingDir: "{app}"

[Tasks]
Name: "addtopath"; Description: "Add Infinity-IaC to your user PATH"; Flags: unchecked

[Registry]
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; ValueData: "{code:GetUpdatedPath}"; Flags: preservestringtype; Check: WizardIsTaskSelected('addtopath')
Root: HKCU; Subkey: "Software\Infinity-IaC"; ValueType: dword; ValueName: "PathManaged"; ValueData: "1"; Flags: uninsdeletekey; Check: PathWasAdded

[Code]
var
  PathAdded: Boolean;

function PathContains(const PathValue, Entry: string): Boolean;
begin
  Result := Pos(';' + Lowercase(Entry) + ';', ';' + Lowercase(PathValue) + ';') > 0;
end;

function GetUpdatedPath(Param: String): String;
var
  CurrentPath, InstallPath: String;
begin
  InstallPath := ExpandConstant('{app}');
  if RegQueryStringValue(HKCU, 'Environment', 'Path', CurrentPath) then
  begin
    if PathContains(CurrentPath, InstallPath) then
      Result := CurrentPath
    else
    begin
      Result := CurrentPath + ';' + InstallPath;
      PathAdded := True;
    end;
  end
  else
  begin
    Result := InstallPath;
    PathAdded := True;
  end;
end;

function PathWasAdded: Boolean;
begin
  Result := PathAdded;
end;

function RemovePathEntry(const PathValue, Entry: String): String;
var
  Parts, Part: String;
  I: Integer;
  First: Boolean;
begin
  Result := '';
  Parts := PathValue;
  First := True;
  while True do
  begin
    I := Pos(';', Parts);
    if I = 0 then
      Part := Parts
    else
      Part := Copy(Parts, 1, I - 1);
    if CompareText(Part, Entry) <> 0 then
    begin
      if not First then Result := Result + ';';
      Result := Result + Part;
      First := False;
    end;
    if I = 0 then Break;
    Delete(Parts, 1, I);
  end;
end;

procedure RemoveFromUserPath;
var
  CurrentPath, NewPath: String;
  PathManaged: Cardinal;
begin
  if RegQueryDWordValue(HKCU, 'Software\Infinity-IaC', 'PathManaged', PathManaged) and
     (PathManaged = 1) and
     RegQueryStringValue(HKCU, 'Environment', 'Path', CurrentPath) then
  begin
    NewPath := RemovePathEntry(CurrentPath, ExpandConstant('{app}'));
    RegWriteExpandStringValue(HKCU, 'Environment', 'Path', NewPath);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    RemoveFromUserPath;
end;
