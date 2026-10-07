# Artifacts verification ae54a95da802

Classification: `none`

## Steps

- [x] doctor (3 ms) http://127.0.0.1:58914
- [x] REST create repository (14 ms) repository=agent-ae54a95da802/core-ae54a95da802
- [x] REST commit (31 ms) sha=53a873fc95778b06d3c22229f54c2f5ba1f602d2 sequence=1
- [x] REST read (3 ms) path=README.md
- [x] create Git credential (0 ms) scope=write
- [x] Git clone (59 ms) cloned repository
- [x] Git verify REST commit (12 ms) head=53a873fc95778b06d3c22229f54c2f5ba1f602d2
- [x] Git commit and push (112 ms) sha=a6931eee39746ae4c05f3d6df182ab4d3a103659
- [x] REST readback (3 ms) path=result.txt
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

- `git.log` `15d3f38169b12b8a7097687e53120fda0f1da25faab0ecfce187861bec4e16f4` (435 bytes)
