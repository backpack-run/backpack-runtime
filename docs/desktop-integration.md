# External client boundary

Backpack Runtime is a standalone product. Any external application should call its loopback HTTP API rather than execute the CLI or take ownership of engine processes. Model installation/checksums, package discovery, runtime installation, hardware inspection, process/session lifecycle, inference, and SSH execution belong to Runtime. Product UI, conversation storage, workspaces, and application-specific state belong to the client.

Health/version, model listing, session CRUD, streaming chat, transcription, and speech use a generic API contract. API clients must receive structured progress/events rather than terminal-formatted strings; that event surface remains in progress. `pkg/client` is the reference Go client.
