# Artifacts verification c4430a33d470

Classification: `assertion`

Failure: multipart commit accepted

## Steps

- [x] doctor (3 ms) http://127.0.0.1:57858
- [x] REST create repository (16 ms) repository=agent-c4430a33d470/core-c4430a33d470
- [x] REST commit (77 ms) sha=c2d008377e178ff96daf37fc93e11b52f11a849c sequence=1
- [x] REST read (16 ms) path=README.md
- [x] create Git credential (2 ms) scope=write
- [x] Git clone (93 ms) cloned repository
- [x] Git verify REST commit (15 ms) head=c2d008377e178ff96daf37fc93e11b52f11a849c
- [x] Git commit and push (305 ms) sha=0d75d0add719f990454d23faeb26088d0551ebd2
- [x] REST readback (11 ms) path=result.txt
- [x] refs and WAL visibility (4 ms) refs=2 wal=2
- [x] generate binary fixture (1357 ms) bytes=536870865 sha256=a5eec1cbc47df9fe391a8ea31f2ab9bed998a4b86f79373bf2b06666f1707fac
- [ ] multipart commit (34774 ms) sha= bytes=536870912 sequence=0
- [x] delete repository (7 ms) final server state checked

## Assertions

- [x] repository has Git remote remote present
- [x] REST content matches README.md content matched
- [x] Git sees REST content README matched
- [x] Git sees REST SHA SHA matched
- [x] REST content matches result.txt content matched
- [x] main ref exposes Git push refs/heads/main matched
- [x] WAL exposes REST and Git publications at least two publications
- [ ] multipart commit accepted HTTP 201
- [x] repository absent after cleanup GET returned not found

## Artifacts

- `git.log` `e2bd81e99849d8bc00abc6d5df001722898cf505bb03ed245d7345c347720977` (434 bytes)
