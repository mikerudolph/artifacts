# Artifacts verification 6a889da5b8db

Classification: `none`

## Steps

- [x] doctor (11 ms) http://127.0.0.1:57858
- [x] REST create repository (12 ms) repository=agent-6a889da5b8db/core-6a889da5b8db
- [x] REST commit (53 ms) sha=9bdfac772ffa3e1297d6cfce1f6861f9d4f7ed41 sequence=1
- [x] REST read (5 ms) path=README.md
- [x] create Git credential (1 ms) scope=write
- [x] Git clone (76 ms) cloned repository
- [x] Git verify REST commit (12 ms) head=9bdfac772ffa3e1297d6cfce1f6861f9d4f7ed41
- [x] Git commit and push (223 ms) sha=57b028a88bbfc1766629e69f283948cb8cd2c98f
- [x] REST readback (6 ms) path=result.txt
- [x] refs and WAL visibility (2 ms) refs=2 wal=2
- [x] delete repository (5 ms) final server state checked

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

- `git.log` `9605b782efaef6145f3290f6e0ba4da0123bf1f72568d0ffc231a45b390e7a18` (435 bytes)
