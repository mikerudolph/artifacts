# Artifacts verification 9f90a2a9ee69

Classification: `none`

## Steps

- [x] doctor (5 ms) http://127.0.0.1:61708
- [x] REST create repository (11 ms) repository=agent-9f90a2a9ee69/core-9f90a2a9ee69
- [x] REST commit (39 ms) sha=88b9b13f631d1f5b485f6198745194310a2d62fc sequence=1
- [x] REST read (7 ms) path=README.md
- [x] create Git credential (2 ms) scope=write
- [x] Git clone (116 ms) cloned repository
- [x] Git verify REST commit (20 ms) head=88b9b13f631d1f5b485f6198745194310a2d62fc
- [x] Git commit and push (380 ms) sha=3967d0bc5b9d602ff678d319bb4cf0090a4609f8
- [x] REST readback (4 ms) path=result.txt
- [x] refs and WAL visibility (4 ms) refs=2 wal=2
- [x] delete repository (8 ms) final server state checked

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

- `git.log` `e1982a82c35ec2b6d38bd27ac3f2cb2707d42dc6c277a0b730df4817cd1a7886` (435 bytes)
