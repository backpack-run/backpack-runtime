# Compute targets

A compute target answers where work runs; it does not know model families. Local execution implements hardware inspection and managed process launch. SSH and managed Backpack Compute currently expose explicit boundaries but return honest not-implemented errors. SSH will add verified host identity, target inspection, runtime bootstrap, checksum-aware synchronization, remote lifecycle, tunnel/log streaming, and reconnection without changing llama.cpp adapter selection.

