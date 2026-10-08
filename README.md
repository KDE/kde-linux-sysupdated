<!--
    SPDX-License-Identifier: CC0-1.0
    SPDX-FileCopyrightText: 2025 Harald Sitter <sitter@kde.org>
-->

# KDE Linux sysupdated 🐧

Faster updates through fewer downloads!

A special daemon to sit between systemd and the actual update mirrors.
It turns erofs downloads into delta downloads by using the server-side caibx file as chunk index and existing erofs
files on disk as seed files.

Heavily based on the excellent project desync; functionally inspired by zsync.

## Use

Make sure you have a go toolchain installed! `snap install --classic go` is very easy.
Alternatively you can deploy go to your home https://go.dev/doc/install

```
git clone https://invent.kde.org/sitter/kde-linux-sysupdated.git
cd kde-linux-sysupdated
DESTDIR=~/kde make install
sudo systemd-sysext refresh
sudo systemctl enable --now kde-linux-sysupdated.socket
# use regular update tooling
```

## How it works

- sets up a local HTTP daemon
- redirects the update server via /run/sysupdate.d to the local daemon
- inside the daemon it either redirects calls to the mirror or (in the case of erofs) handles them internally
- when internally handling things it uses desync to chunk up local erofs files and use them as "seed" for the download
- download the chunk index from the server to use the remote file as primary seed
- runs through all chunks in the new erofs and looks for chunks that are already availabe in one of the seed files
  i.e. chunks that are already on-disk will be picked out of the on-disk files rather than downloaded
- all other chunks are downloaded from the mirror
- lastly it sequentially streams all data into the client

## Caveats

This is somewhat inferior to the scenario that is being planned upstream in systemd
because we need to sequentially stream the data we can't write data concurrently to disk directly,
and have to live with IO overhead blocking the stream (e.g. when we are waiting for the mirror to reply).

The current desync API is a bit restrictive and required some re-implementation on our end to fully support this feature.
Also some code copy.

## Remote Control

The update server may control how an update is performed by sending the following headers on a HEAD request for the relevant erofs.

- X-KDE-Delta: when set to false we don't do any delta updates (full file download)
- X-KDE-Store: when set to false we don't use the desync store (may still do HTTP range requests)
