# Artifacts verification 3714f15a062f

Classification: `none`

## Steps

- [x] doctor (4 ms) http://127.0.0.1:50507
- [x] REST create repository (15 ms) repository=agent-3714f15a062f/core-3714f15a062f
- [x] REST commit (193 ms) sha=2e7469b8467776977b8dfd73d9754f5c4374831d sequence=1
- [x] REST read (23 ms) path=README.md
- [x] create Git credential (5 ms) scope=write
- [x] Git clone (115 ms) cloned repository
- [x] Git verify REST commit (9 ms) head=2e7469b8467776977b8dfd73d9754f5c4374831d
- [x] Git commit and push (291 ms) sha=ed5e79ba18831953e2a95b0b06f9c6598eb8dc1a
- [x] REST readback (25 ms) path=result.txt
- [x] refs and WAL visibility (8 ms) refs=2 wal=2
- [x] delete repository (14 ms) final server state checked

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

- `git.log` `9a54a1de29b655c582164011e7c4d916dde2a0b3bfbc29c98f62ea78ff78ee38` (435 bytes)
