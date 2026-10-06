# Artifacts verification ccc906391079

Classification: `none`

## Steps

- [x] doctor (2 ms) http://127.0.0.1:63590
- [x] REST create repository (11 ms) repository=agent-ccc906391079/core-ccc906391079
- [x] REST commit (49 ms) sha=04964bc65f09646b9874c9e7d4fedee39fe31c0c sequence=1
- [x] REST read (19 ms) path=README.md
- [x] create Git credential (3 ms) scope=write
- [x] Git clone (118 ms) cloned repository
- [x] Git verify REST commit (31 ms) head=04964bc65f09646b9874c9e7d4fedee39fe31c0c
- [x] Git commit and push (309 ms) sha=9d99815631a3b597e6245591938d34b748a549e3
- [x] REST readback (5 ms) path=result.txt
- [x] refs and WAL visibility (3 ms) refs=2 wal=2
- [x] delete repository (6 ms) final server state checked

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

- `git.log` `4428b213cba38b27ed0d213a8d1def1e8a4e19888a4bb092b3e4de02f67485a1` (435 bytes)
