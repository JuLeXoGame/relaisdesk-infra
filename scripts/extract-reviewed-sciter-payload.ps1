param(
    [Parameter(Mandatory)][string]$Portable,
    [Parameter(Mandatory)][ValidatePattern('^[a-fA-F0-9]{64}$')][string]$ExpectedSHA256,
    [Parameter(Mandatory)][string]$Destination
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$inputFile = Get-Item -LiteralPath $Portable
if ($inputFile.PSIsContainer -or $inputFile.Length -gt 67108864) { throw 'Invalid portable input.' }
if ((Get-FileHash -LiteralPath $inputFile.FullName).Hash -ne $ExpectedSHA256) { throw 'Portable SHA-256 does not match the reviewed release.' }
if (Test-Path -LiteralPath $Destination) { throw 'Use a new extraction directory.' }

# Decode the reviewed build artifact as data, without executing the portable or
# trusting anything in LOCALAPPDATA. MD5 is only a format check; SHA-256 above
# authenticates the input against the independently selected release identity.
if (-not ('RelaisDeskBuild.SciterPackage' -as [type])) {
Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.IO;
using System.IO.Compression;
using System.Security.Cryptography;
using System.Text;
namespace RelaisDeskBuild {
 public static class SciterPackage {
  static readonly byte[] Marker = Encoding.ASCII.GetBytes("rustdesk");
  static bool Match(byte[] data, int at, byte[] value) {
   if (at < 0 || at + value.Length > data.Length) return false;
   for (int n=0; n<value.Length; n++) if(data[at+n]!=value[n]) return false;
   return true;
  }
  static int Length(byte[] data, ref int at, int max) {
   if(at<0 || at+4>data.Length) throw new InvalidDataException();
   uint n=((uint)data[at]<<24)|((uint)data[at+1]<<16)|((uint)data[at+2]<<8)|data[at+3]; at+=4;
   if(n==0 || n>max || at+(long)n>data.Length) throw new InvalidDataException();
   return (int)n;
  }
  static Dictionary<string,byte[]> Read(byte[] data,int offset) {
   int at=offset+8;
   var files=new Dictionary<string,byte[]>(StringComparer.Ordinal);
   while(!Match(data,at,Marker)) {
    if(files.Count>=3) throw new InvalidDataException();
    int n=Length(data,ref at,128);
    string path=Encoding.UTF8.GetString(data,at,n).Replace('\\','/'); at+=n;
    if(path!="./rustdesk.exe" && path!="./sciter.dll" && path!="./dylib_virtual_display.dll") throw new InvalidDataException();
    string name=path.Substring(2);
    int count=Length(data,ref at,67108864);
    byte[] raw;
    using(var input=new MemoryStream(data,at,count))
    using(var stream=new BrotliStream(input,CompressionMode.Decompress))
    using(var output=new MemoryStream()) {
     byte[] buffer=new byte[65536]; int got;
     while((got=stream.Read(buffer,0,buffer.Length))>0) {
      if(output.Length+got>67108864) throw new InvalidDataException();
      output.Write(buffer,0,got);
     }
     raw=output.ToArray();
    }
    at+=count;
    if(at+32>data.Length) throw new InvalidDataException();
    string md5=Encoding.ASCII.GetString(data,at,32); at+=32;
    if(!String.Equals(Convert.ToHexString(MD5.HashData(raw)),md5,StringComparison.OrdinalIgnoreCase)) throw new InvalidDataException();
    if(raw.Length<64 || raw[0]!=0x4d || raw[1]!=0x5a) throw new InvalidDataException();
    int pe=BitConverter.ToInt32(raw,0x3c);
    if(pe<0 || pe+6>raw.Length || BitConverter.ToUInt32(raw,pe)!=0x00004550 || BitConverter.ToUInt16(raw,pe+4)!=0x8664) throw new InvalidDataException();
    files.Add(name,raw);
   }
   if(files.Count!=3 || !Match(data,at+8,Encoding.ASCII.GetBytes(".\\rustdesk.exe")) && !Match(data,at+8,Encoding.ASCII.GetBytes("./rustdesk.exe"))) throw new InvalidDataException();
   return files;
  }
  public static Dictionary<string,byte[]> Extract(byte[] data) {
   Dictionary<string,byte[]> found=null;
   for(int at=0;at<data.Length-12;at++) {
    if(!Match(data,at,Marker) || data[at+8]!=0 || data[at+9]!=0 || data[at+10]!=0) continue;
    Dictionary<string,byte[]> current;
    try { current=Read(data,at); } catch (InvalidDataException) { continue; } catch (ArgumentException) { continue; }
    if(found!=null) throw new InvalidDataException("Ambiguous native package");
    found=current;
   }
   return found ?? throw new InvalidDataException("Complete native Sciter payload not found");
  }
 }
}
'@
}
$payload = [RelaisDeskBuild.SciterPackage]::Extract([IO.File]::ReadAllBytes($inputFile.FullName))
$destinationPath = [IO.Path]::GetFullPath($Destination)
New-Item -ItemType Directory -Path $destinationPath | Out-Null
foreach ($name in $payload.Keys) {
    $path = Join-Path $destinationPath $name
    [IO.File]::WriteAllBytes($path, $payload[$name])
    [PSCustomObject]@{ Name=$name; Bytes=$payload[$name].Length; SHA256=(Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant() }
}
