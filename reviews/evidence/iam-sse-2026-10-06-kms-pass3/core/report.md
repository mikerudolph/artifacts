# Artifacts verification f7e599b7f0dc

Classification: `none`

## Steps

- [x] doctor (2 ms) http://127.0.0.1:58799
- [x] REST create repository (10 ms) repository=agent-f7e599b7f0dc/core-f7e599b7f0dc
- [x] REST commit (40 ms) sha=4336b1c7b7c3fb8b1933c5ac4196eb719aba1027 sequence=1
- [x] REST read (4 ms) path=README.md
- [x] create Git credential (2 ms) scope=write
- [x] Git clone (69 ms) cloned repository
- [x] Git verify REST commit (10 ms) head=4336b1c7b7c3fb8b1933c5ac4196eb719aba1027
- [x] Git commit and push (114 ms) sha=2b8ea6dd82cd1390eb17b6d8ef6bc426538d2ef8
- [x] REST readback (2 ms) path=result.txt
- [x] refs and WAL visibility (1 ms) refs=2 wal=2
- [x] delete repository (3 ms) final server state checked

## Assertions

- [x] repository has Git remote remote present
- [x] REST content matches README.md content matched
- [x] Git sees REST content README matched
- [x] Git sees REST SHA SHA matched
- [x] REST content matches result.txt content matched
- [x] main ref exposes Git push refs/heads/main matched
- [x] WAL exposes REST and Git publications at least two publications
- [x] repository absent after cleanup GET returned not found

## Artifacts

- `git.log` `310bc4a3d3e9e566e9384c86dbbaabf21704cfd70f007852f4514f3d144c53c6` (435 bytes)
