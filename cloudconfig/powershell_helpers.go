// Copyright 2014, 2015 Canonical Ltd.
// Copyright 2014, 2015 Cloudbase Solutions
// Copyright 2012 Aaron Jensen
// Copyright (c) 2009, Vladimir Vasiltsov All rights reserved.
//
// Licensed under the AGPLv3, see LICENCE file for details.
//
// This file borrowed some code from https://bitbucket.org/splatteredbits/carbon
// (see Source/Security/Privilege.cs). This external source is licensed under
// Apache-2.0 license which is compatible with AGPLv3 license. Because it's
// compatible we can and have licensed this derived work under AGPLv3. The original
// Apache-2.0 license for the external source can be found inside Apache-License.txt.
// Copyright statement of the external source: Copyright 2012 Aaron Jensen
//
// This file borrowed some code from https://code.google.com/p/tar-cs/ which is
// This external source is licensed under BSD3 License which is compatible with
// AGPLv3 license. Because it's compatible we can have have licensed this
// derived work under AGPLv3. The original BSD3 license for the external source
// can be found inside BSD3-License.txt.
// Copyright statement of the external source:
// Copyright (c) 2009, Vladimir Vasiltsov All rights reserved.

package cloudconfig

var winPowershellHelperFunctions = `

$ErrorActionPreference = "Stop"

function ExecRetry($command, $retryInterval = 15)
{
	$currErrorActionPreference = $ErrorActionPreference
	$ErrorActionPreference = "Continue"

	while ($true)
	{
		try
		{
			& $command
			break
		}
		catch [System.Exception]
		{
			Write-Error $_.Exception
			Start-Sleep $retryInterval
		}
	}

	$ErrorActionPreference = $currErrorActionPreference
}

function create-account ([string]$accountName, [string]$accountDescription, [string]$password) {
	$hostname = hostname
	$comp = [adsi]"WinNT://$hostname"
	$user = $comp.Create("User", $accountName)
	$user.SetPassword($password)
	$user.SetInfo()
	$user.description = $accountDescription
	$user.SetInfo()
	$User.UserFlags[0] = $User.UserFlags[0] -bor 0x10000
	$user.SetInfo()

	# This gets the Administrator group name that is localized on different windows versions. 
	# However the SID S-1-5-32-544 is the same on all versions.
	$adminGroup = (New-Object System.Security.Principal.SecurityIdentifier("S-1-5-32-544")).Translate([System.Security.Principal.NTAccount]).Value.Split("\")[1]

	$objOU = [ADSI]"WinNT://$hostname/$adminGroup,group"
	$objOU.add("WinNT://$hostname/$accountName")
}

$Source = @"
using System;
using System.Text;
using System.Runtime.InteropServices;

namespace PSCloudbase
{
	public sealed class Win32CryptApi
	{
		public static long CRYPT_SILENT = 0x00000040;
		public static long CRYPT_VERIFYCONTEXT = 0xF0000000;
		public static int PROV_RSA_FULL = 1;

		[DllImport("advapi32.dll", CharSet=CharSet.Auto, SetLastError=true)]
		[return : MarshalAs(UnmanagedType.Bool)]
		public static extern bool CryptAcquireContext(ref IntPtr hProv,
													  StringBuilder pszContainer, // Don't use string, as Powershell replaces $null with an empty string
													  StringBuilder pszProvider, // Don't use string, as Powershell replaces $null with an empty string
													  uint dwProvType,
													  uint dwFlags);

		[DllImport("Advapi32.dll", EntryPoint = "CryptReleaseContext", CharSet = CharSet.Unicode, SetLastError = true)]
		public static extern bool CryptReleaseContext(IntPtr hProv, Int32 dwFlags);

		[DllImport("advapi32.dll", SetLastError=true)]
		public static extern bool CryptGenRandom(IntPtr hProv, uint dwLen, byte[] pbBuffer);

		[DllImport("Kernel32.dll")]
		public static extern uint GetLastError();
	}
}
"@

Add-Type -TypeDefinition $Source -Language CSharp

function Get-RandomPassword
{
	[CmdletBinding()]
	param
	(
		[parameter(Mandatory=$true)]
		[int]$Length
	)
	process
	{
		$hProvider = 0
		try
		{
			if(![PSCloudbase.Win32CryptApi]::CryptAcquireContext([ref]$hProvider, $null, $null,
																 [PSCloudbase.Win32CryptApi]::PROV_RSA_FULL,
																 ([PSCloudbase.Win32CryptApi]::CRYPT_VERIFYCONTEXT -bor
																  [PSCloudbase.Win32CryptApi]::CRYPT_SILENT)))
			{
				throw "CryptAcquireContext failed with error: 0x" + "{0:X0}" -f [PSCloudbase.Win32CryptApi]::GetLastError()
			}

			$buffer = New-Object byte[] $Length
			if(![PSCloudbase.Win32CryptApi]::CryptGenRandom($hProvider, $Length, $buffer))
			{
				throw "CryptGenRandom failed with error: 0x" + "{0:X0}" -f [PSCloudbase.Win32CryptApi]::GetLastError()
			}

			$buffer | ForEach-Object { $password += "{0:X0}" -f $_ }
			return $password
		}
		finally
		{
			if($hProvider)
			{
				$retVal = [PSCloudbase.Win32CryptApi]::CryptReleaseContext($hProvider, 0)
			}
		}
	}
}

$SourcePolicy = @"
/*
Original sources available at: https://bitbucket.org/splatteredbits/carbon
*/

using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;

namespace PSCarbon
{
	public sealed class Lsa
	{
		// ReSharper disable InconsistentNaming
		[StructLayout(LayoutKind.Sequential)]
		internal struct LSA_UNICODE_STRING
		{
			internal LSA_UNICODE_STRING(string inputString)
			{
				if (inputString == null)
				{
					Buffer = IntPtr.Zero;
					Length = 0;
					MaximumLength = 0;
				}
				else
				{
					Buffer = Marshal.StringToHGlobalAuto(inputString);
					Length = (ushort)(inputString.Length * UnicodeEncoding.CharSize);
					MaximumLength = (ushort)((inputString.Length + 1) * UnicodeEncoding.CharSize);
				}
			}

			internal ushort Length;
			internal ushort MaximumLength;
			internal IntPtr Buffer;
		}

		[StructLayout(LayoutKind.Sequential)]
		internal struct LSA_OBJECT_ATTRIBUTES
		{
			internal uint Length;
			internal IntPtr RootDirectory;
			internal LSA_UNICODE_STRING ObjectName;
			internal uint Attributes;
			internal IntPtr SecurityDescriptor;
			internal IntPtr SecurityQualityOfService;
		}

		[StructLayout(LayoutKind.Sequential)]
		public struct LUID
		{
			public uint LowPart;
			public int HighPart;
		}

		// ReSharper disable UnusedMember.Local
		private const uint POLICY_VIEW_LOCAL_INFORMATION = 0x00000001;
		private const uint POLICY_VIEW_AUDIT_INFORMATION = 0x00000002;
		private const uint POLICY_GET_PRIVATE_INFORMATION = 0x00000004;
		private const uint POLICY_TRUST_ADMIN = 0x00000008;
		private const uint POLICY_CREATE_ACCOUNT = 0x00000010;
		private const uint POLICY_CREATE_SECRET = 0x00000014;
		private const uint POLICY_CREATE_PRIVILEGE = 0x00000040;
		private const uint POLICY_SET_DEFAULT_QUOTA_LIMITS = 0x00000080;
		private const uint POLICY_SET_AUDIT_REQUIREMENTS = 0x00000100;
		private const uint POLICY_AUDIT_LOG_ADMIN = 0x00000200;
		private const uint POLICY_SERVER_ADMIN = 0x00000400;
		private const uint POLICY_LOOKUP_NAMES = 0x00000800;
		private const uint POLICY_NOTIFICATION = 0x00001000;
		// ReSharper restore UnusedMember.Local

		[DllImport("advapi32.dll", CharSet = CharSet.Auto, SetLastError = true)]
		public static extern bool LookupPrivilegeValue(
			[MarshalAs(UnmanagedType.LPTStr)] string lpSystemName,
			[MarshalAs(UnmanagedType.LPTStr)] string lpName,
			out LUID lpLuid);

		[DllImport("advapi32.dll", CharSet = CharSet.Unicode)]
		private static extern uint LsaAddAccountRights(
			IntPtr PolicyHandle,
			IntPtr AccountSid,
			LSA_UNICODE_STRING[] UserRights,
			uint CountOfRights);

		[DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = false)]
		private static extern uint LsaClose(IntPtr ObjectHandle);

		[DllImport("advapi32.dll", SetLastError = true)]
		private static extern uint LsaEnumerateAccountRights(IntPtr PolicyHandle,
			IntPtr AccountSid,
			out IntPtr UserRights,
			out uint CountOfRights
			);

		[DllImport("advapi32.dll", SetLastError = true)]
		private static extern uint LsaFreeMemory(IntPtr pBuffer);

		[DllImport("advapi32.dll")]
		private static extern int LsaNtStatusToWinError(long status);

		[DllImport("advapi32.dll", SetLastError = true, PreserveSig = true)]
		private static extern uint LsaOpenPolicy(ref LSA_UNICODE_STRING SystemName, ref LSA_OBJECT_ATTRIBUTES ObjectAttributes, uint DesiredAccess, out IntPtr PolicyHandle );

		[DllImport("advapi32.dll", SetLastError = true, PreserveSig = true)]
		static extern uint LsaRemoveAccountRights(
			IntPtr PolicyHandle,
			IntPtr AccountSid,
			[MarshalAs(UnmanagedType.U1)]
			bool AllRights,
			LSA_UNICODE_STRING[] UserRights,
			uint CountOfRights);
		// ReSharper restore InconsistentNaming

		private static IntPtr GetIdentitySid(string identity)
		{
			SecurityIdentifier sid =
				new NTAccount(identity).Translate(typeof (SecurityIdentifier)) as SecurityIdentifier;
			if (sid == null)
			{
				throw new ArgumentException(string.Format("Account {0} not found.", identity));
			}
			byte[] sidBytes = new byte[sid.BinaryLength];
			sid.GetBinaryForm(sidBytes, 0);
			System.IntPtr sidPtr = Marshal.AllocHGlobal(sidBytes.Length);
			Marshal.Copy(sidBytes, 0, sidPtr, sidBytes.Length);
			return sidPtr;
		}

		private static IntPtr GetLsaPolicyHandle()
		{
			string computerName = Environment.MachineName;
			IntPtr hPolicy;
			LSA_OBJECT_ATTRIBUTES objectAttributes = new LSA_OBJECT_ATTRIBUTES();
			objectAttributes.Length = 0;
			objectAttributes.RootDirectory = IntPtr.Zero;
			objectAttributes.Attributes = 0;
			objectAttributes.SecurityDescriptor = IntPtr.Zero;
			objectAttributes.SecurityQualityOfService = IntPtr.Zero;

			const uint ACCESS_MASK = POLICY_CREATE_SECRET | POLICY_LOOKUP_NAMES | POLICY_VIEW_LOCAL_INFORMATION;
			LSA_UNICODE_STRING machineNameLsa = new LSA_UNICODE_STRING(computerName);
			uint result = LsaOpenPolicy(ref machineNameLsa, ref objectAttributes, ACCESS_MASK, out hPolicy);
			HandleLsaResult(result);
			return hPolicy;
		}

		public static string[] GetPrivileges(string identity)
		{
			IntPtr sidPtr = GetIdentitySid(identity);
			IntPtr hPolicy = GetLsaPolicyHandle();
			IntPtr rightsPtr = IntPtr.Zero;

			try
			{

				List<string> privileges = new List<string>();

				uint rightsCount;
				uint result = LsaEnumerateAccountRights(hPolicy, sidPtr, out rightsPtr, out rightsCount);
				int win32ErrorCode = LsaNtStatusToWinError(result);
				// the user has no privileges
				if( win32ErrorCode == STATUS_OBJECT_NAME_NOT_FOUND )
				{
					return new string[0];
				}
				HandleLsaResult(result);

				LSA_UNICODE_STRING myLsaus = new LSA_UNICODE_STRING();
				for (ulong i = 0; i < rightsCount; i++)
				{
					IntPtr itemAddr = new IntPtr(rightsPtr.ToInt64() + (long) (i*(ulong) Marshal.SizeOf(myLsaus)));
					myLsaus = (LSA_UNICODE_STRING) Marshal.PtrToStructure(itemAddr, myLsaus.GetType());
					char[] cvt = new char[myLsaus.Length/UnicodeEncoding.CharSize];
					Marshal.Copy(myLsaus.Buffer, cvt, 0, myLsaus.Length/UnicodeEncoding.CharSize);
					string thisRight = new string(cvt);
					privileges.Add(thisRight);
				}
				return privileges.ToArray();
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				uint result = LsaClose(hPolicy);
				HandleLsaResult(result);
				result = LsaFreeMemory(rightsPtr);
				HandleLsaResult(result);
			}
		}

		public static void GrantPrivileges(string identity, string[] privileges)
		{
			IntPtr sidPtr = GetIdentitySid(identity);
			IntPtr hPolicy = GetLsaPolicyHandle();

			try
			{
				LSA_UNICODE_STRING[] lsaPrivileges = StringsToLsaStrings(privileges);
				uint result = LsaAddAccountRights(hPolicy, sidPtr, lsaPrivileges, (uint)lsaPrivileges.Length);
				HandleLsaResult(result);
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				uint result = LsaClose(hPolicy);
				HandleLsaResult(result);
			}
		}

		const int STATUS_SUCCESS = 0x0;
		const int STATUS_OBJECT_NAME_NOT_FOUND = 0x00000002;
		const int STATUS_ACCESS_DENIED = 0x00000005;
		const int STATUS_INVALID_HANDLE = 0x00000006;
		const int STATUS_UNSUCCESSFUL = 0x0000001F;
		const int STATUS_INVALID_PARAMETER = 0x00000057;
		const int STATUS_NO_SUCH_PRIVILEGE = 0x00000521;
		const int STATUS_INVALID_SERVER_STATE = 0x00000548;
		const int STATUS_INTERNAL_DB_ERROR = 0x00000567;
		const int STATUS_INSUFFICIENT_RESOURCES = 0x000005AA;

		private static Dictionary<int, string> ErrorMessages = new Dictionary<int, string>();
		public Lsa () {
			ErrorMessages.Add(STATUS_ACCESS_DENIED, "Access denied. Caller does not have the appropriate access to complete the operation.");
			ErrorMessages.Add(STATUS_INVALID_HANDLE, "Invalid handle. Indicates an object or RPC handle is not valid in the context used.");
			ErrorMessages.Add(STATUS_UNSUCCESSFUL, "Unsuccessful. Generic failure, such as RPC connection failure.");
			ErrorMessages.Add(STATUS_INVALID_PARAMETER, "Invalid parameter. One of the parameters is not valid.");
			ErrorMessages.Add(STATUS_NO_SUCH_PRIVILEGE, "No such privilege. Indicates a specified privilege does not exist.");
			ErrorMessages.Add(STATUS_INVALID_SERVER_STATE, "Invalid server state. Indicates the LSA server is currently disabled.");
			ErrorMessages.Add(STATUS_INTERNAL_DB_ERROR, "Internal database error. The LSA database contains an internal inconsistency.");
			ErrorMessages.Add(STATUS_INSUFFICIENT_RESOURCES, "Insufficient resources. There are not enough system resources (such as memory to allocate buffers) to complete the call.");
			ErrorMessages.Add(STATUS_OBJECT_NAME_NOT_FOUND, "Object name not found. An object in the LSA policy database was not found. The object may have been specified either by SID or by name, depending on its type.");
		}

		private static void HandleLsaResult(uint returnCode)
		{
			int win32ErrorCode = LsaNtStatusToWinError(returnCode);

			if( win32ErrorCode == STATUS_SUCCESS)
				return;

			if( ErrorMessages.ContainsKey(win32ErrorCode) )
			{
				throw new Win32Exception(win32ErrorCode, ErrorMessages[win32ErrorCode]);
			}

			throw new Win32Exception(win32ErrorCode);
		}

		public static void RevokePrivileges(string identity, string[] privileges)
		{
			IntPtr sidPtr = GetIdentitySid(identity);
			IntPtr hPolicy = GetLsaPolicyHandle();

			try
			{
				string[] currentPrivileges = GetPrivileges(identity);
				if (currentPrivileges.Length == 0)
				{
					return;
				}
				LSA_UNICODE_STRING[] lsaPrivileges = StringsToLsaStrings(privileges);
				uint result = LsaRemoveAccountRights(hPolicy, sidPtr, false, lsaPrivileges, (uint)lsaPrivileges.Length);
				HandleLsaResult(result);
			}
			finally
			{
				Marshal.FreeHGlobal(sidPtr);
				uint result = LsaClose(hPolicy);
				HandleLsaResult(result);
			}

		}

		private static LSA_UNICODE_STRING[] StringsToLsaStrings(string[] privileges)
		{
			LSA_UNICODE_STRING[] lsaPrivileges = new LSA_UNICODE_STRING[privileges.Length];
			for (int idx = 0; idx < privileges.Length; ++idx)
			{
				lsaPrivileges[idx] = new LSA_UNICODE_STRING(privileges[idx]);
			}
			return lsaPrivileges;
		}
	}
}
"@

Add-Type -TypeDefinition $SourcePolicy -Language CSharp

function SetAssignPrimaryTokenPrivilege($UserName)
{
	$privilege = "SeAssignPrimaryTokenPrivilege"
	if (!([PSCarbon.Lsa]::GetPrivileges($UserName) -contains $privilege))
	{
		[PSCarbon.Lsa]::GrantPrivileges($UserName, $privilege)
	}
}

function SetUserLogonAsServiceRights($UserName)
{
	$privilege = "SeServiceLogonRight"
	if (!([PSCarbon.Lsa]::GetPrivileges($UserName) -Contains $privilege))
	{
		[PSCarbon.Lsa]::GrantPrivileges($UserName, $privilege)
	}
}

Function GUnZip-File{
    Param(
        $infile,
        $outdir
        )

    $input = New-Object System.IO.FileStream $inFile, ([IO.FileMode]::Open), ([IO.FileAccess]::Read), ([IO.FileShare]::Read)
    $tempFile = "$env:TEMP\jujud.tar"
    $tempOut = New-Object System.IO.FileStream $tempFile, ([IO.FileMode]::Create), ([IO.FileAccess]::Write), ([IO.FileShare]::None)
    $gzipStream = New-Object System.IO.Compression.GzipStream $input, ([IO.Compression.CompressionMode]::Decompress)

    $buffer = New-Object byte[](1024)
    while($true){
        $read = $gzipstream.Read($buffer, 0, 1024)
        if ($read -le 0){break}
        $tempOut.Write($buffer, 0, $read)
    }
    $gzipStream.Close()
    $tempOut.Close()
    $input.Close()

    $in = New-Object System.IO.FileStream $tempFile, ([IO.FileMode]::Open), ([IO.FileAccess]::Read), ([IO.FileShare]::Read)
    Untar-File $in $outdir
    $in.Close()
    rm $tempFile
}

$HEADERSIZE = 512

Function Untar-File {
    Param(
        $inStream,
        $outdir
        )
	$DirectoryEntryType = 0x35
    $headerBytes = New-Object byte[]($HEADERSIZE)

    # $headerBytes is written inside, function returns whether we've reached the end
    while(GetHeaderBytes $inStream $headerBytes) {
        $fileName, $entryType, $sizeInBytes = GetFileInfoFromHeader $headerBytes

        $totalPath = Join-Path $outDir $fileName
        if ($entryType -eq $DirectoryEntryType) {
            [System.IO.Directory]::CreateDirectory($totalPath)
            continue;
        }

        $fName = [System.IO.Path]::GetFileName($totalPath)
        $dirName = [System.IO.Path]::GetDirectoryName($totalPath)
        [System.IO.Directory]::CreateDirectory($dirName)
        $file = [System.IO.File]::Create($totalPath)
        WriteTarEntryToFile $inStream $file $sizeInBytes
        $file.Close()
    }
}

Function WriteTarEntryToFile {
    Param(
        $inStream,
        $outFile,
        $sizeInBytes
        )
        $moveToAlign512 = 0
        $toRead = 0
        $buf = New-Object byte[](512)

        $remainingBytesInFile = $sizeInBytes
        while ($remainingBytesInFile -ne 0) {
            if ($remainingBytesInFile - 512 -lt 0) {
                $moveToAlign512 = 512 - $remainingBytesInFile
                $toRead = $remainingBytesInFile
            } else {
                $toRead = 512
            }

            $bytesRead = 0
            $bytesRemainingToRead = $toRead
            while ($bytesRead -lt $toRead -and $bytesRemainingToRead -gt 0) {
                $bytesRead = $inStream.Read($buf, $toRead - $bytesRemainingToRead, $bytesRemainingToRead)
                $bytesRemainingToRead = $bytesRemainingToRead - $bytesRead
                $remainingBytesInFile = $remainingBytesInFile - $bytesRead
                $outFile.Write($buf, 0, $bytesRead)
            }

            if ($moveToAlign512 -ne 0) {
                $inStream.Seek($moveToAlign512, [System.IO.SeekOrigin]::Current)
            }
        }
}

Function GetHeaderBytes {
    Param($inStream, $headerBytes)

    $headerRead = 0
    $bytesRemaining = $HEADERSIZE
	while ($bytesRemaining -gt 0)
    {
        $headerRead = $inStream.Read($headerBytes, $HEADERSIZE - $bytesRemaining, $bytesRemaining)
        $bytesRemaining -= $headerRead
        if ($headerRead -le 0 -and $bytesRemaining -gt 0)
        {
            throw "Error reading tar header. Header size invalid"
        }
    }

    # Proper end of archive is 2 empty headers
    if (IsEmptyByteArray $headerBytes) {
        $bytesRemaining = $HEADERSIZE
	    while ($bytesRemaining -gt 0)
        {
            $headerRead = $inStream.Read($headerBytes, $HEADERSIZE - $bytesRemaining, $bytesRemaining)
            $bytesRemaining -= $headerRead
            if ($headerRead -le 0 -and $bytesRemaining -gt 0)
            {
                throw "Broken end archive"
            }
        }
        if ($bytesRemaining -eq 0 -and (IsEmptyByteArray($headerBytes)))
        {
            return $false
        }
        throw "Error occured: expected end of archive"
    }

    return $true
}

Function GetFileInfoFromHeader {
    Param($headerBytes)

    $FileName = [System.Text.Encoding]::UTF8.GetString($headerBytes, 0, 100);
    $EntryType = $headerBytes[156];
    $SizeInBytes = [Convert]::ToInt64([System.Text.Encoding]::ASCII.GetString($headerBytes, 124, 11).Trim(), 8);
    Return $FileName.replace("` + "`" + `0", [String].Empty), $EntryType, $SizeInBytes
}

Function IsEmptyByteArray {
    Param ($bytes)
    foreach($b in $bytes) {
        if ($b -ne 0) {
            return $false
        }
    }
    return $true
}

Function Get-FileSHA256{
	Param(
		$FilePath
	)
	$hash = [Security.Cryptography.HashAlgorithm]::Create( "SHA256" )
	$stream = ([IO.StreamReader]$FilePath).BaseStream
	$res = -join ($hash.ComputeHash($stream) | ForEach { "{0:x2}" -f $_ })
	$stream.Close()
	return $res
}

$juju_passwd = Get-RandomPassword 20
$juju_passwd += "^"
create-account jujud "Juju Admin user" $juju_passwd
$hostname = hostname
$juju_user = "$hostname\jujud"

SetUserLogonAsServiceRights $juju_user
SetAssignPrimaryTokenPrivilege $juju_user

$path = "HKLM:\Software\Microsoft\Windows NT\CurrentVersion\Winlogon\SpecialAccounts\UserList"
if(!(Test-Path $path)){
	New-Item -Path $path -force
}
New-ItemProperty $path -Name "jujud" -Value 0 -PropertyType "DWord"

$secpasswd = ConvertTo-SecureString $juju_passwd -AsPlainText -Force
$jujuCreds = New-Object System.Management.Automation.PSCredential ($juju_user, $secpasswd)

`

var UserdataScript = `#ps1_sysnative
$userdata=@"
%s
"@

Function Decode-Base64 {
	Param(
		$inFile,
		$outFile
	)
	$bufferSize = 9000 # should be a multiplier of 4
	$buffer = New-Object char[] $bufferSize

	$reader = [System.IO.File]::OpenText($inFile)
	$writer = [System.IO.File]::OpenWrite($outFile)

	$bytesRead = 0
	do
	{
		$bytesRead = $reader.Read($buffer, 0, $bufferSize);
		$bytes = [Convert]::FromBase64CharArray($buffer, 0, $bytesRead);
		$writer.Write($bytes, 0, $bytes.Length);
	} while ($bytesRead -eq $bufferSize);

	$reader.Dispose()
	$writer.Dispose()
}

Function GUnZip-File {
	Param(
		$inFile,
		$outFile
	)
	$in = New-Object System.IO.FileStream $inFile, ([IO.FileMode]::Open), ([IO.FileAccess]::Read), ([IO.FileShare]::Read)
	$out = New-Object System.IO.FileStream $outFile, ([IO.FileMode]::Create), ([IO.FileAccess]::Write), ([IO.FileShare]::None)
	$gzipStream = New-Object System.IO.Compression.GZipStream $in, ([IO.Compression.CompressionMode]::Decompress)
	$buffer = New-Object byte[](1024)
	while($true){
		$read = $gzipstream.Read($buffer, 0, 1024)
		if ($read -le 0){break}
		$out.Write($buffer, 0, $read)
	}
	$gzipStream.Close()
	$out.Close()
	$in.Close()
}

$b64File = "$env:TEMP\juju\udata.b64"
$gzFile = "$env:TEMP\juju\udata.gz"
$udataScript = "$env:TEMP\juju\udata.ps1"
mkdir "$env:TEMP\juju"

Set-Content $b64File $userdata
Decode-Base64 -inFile $b64File -outFile $gzFile
GUnZip-File -inFile $gzFile -outFile $udataScript

& $udataScript

rm -Recurse "$env:TEMP\juju"
`
