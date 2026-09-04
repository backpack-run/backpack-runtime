# Desktop integration

Desktop should call the loopback Backpack Runtime API, not execute the CLI. Model installation/checksums, package discovery, runtime installation, hardware inspection, process/session lifecycle, engine endpoints, and SSH execution move to Runtime. Conversation storage, interface state, audio capture, file pickers, notifications, and presentation stay in Desktop.

Migration should proceed endpoint-by-endpoint: health/version and models first; pulls and progress events second; local chat sessions third; voice/media jobs and SSH last. During transition, Desktop can detect the runtime API and retain its existing path as a compatibility fallback without sharing mutable process state.

