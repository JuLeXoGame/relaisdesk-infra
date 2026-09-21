Native Windows service payload is populated by the release build scripts from
the reviewed RustDesk native build (rustdesk.exe, sciter.dll and runtime DLLs).
This placeholder contains no executable. Permanent installation fails closed
when the native payload or its pinned SHA-256 is missing.
