# Artifacts verification 803f715f56a1

Classification: `none`

## Steps

- [x] doctor (1 ms) http://127.0.0.1:58922
- [x] REST create repository (4 ms) repository=agent-803f715f56a1/core-803f715f56a1
- [x] REST commit (17 ms) sha=53a873fc95778b06d3c22229f54c2f5ba1f602d2 sequence=1
- [x] REST read (2 ms) path=README.md
- [x] create Git credential (1 ms) scope=write
- [x] Git clone (55 ms) cloned repository
- [x] Git verify REST commit (10 ms) head=53a873fc95778b06d3c22229f54c2f5ba1f602d2
- [x] Git commit and push (94 ms) sha=a6931eee39746ae4c05f3d6df182ab4d3a103659
- [x] REST readback (3 ms) path=result.txt
- [x] refs and WAL visibility (1 ms) refs=2 wal=2
- [x] delete repository (4 ms) final server state checked

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

- `git.log` `76c82b9e5e45b3aa1243d74c6d37bc553a82bed721d05b065ed16bdca7bf9450` (435 bytes)
