# Artifacts verification c2dbeedbce96

Classification: `none`

## Steps

- [x] doctor (1 ms) http://127.0.0.1:49591
- [x] REST create repository (6 ms) repository=agent-c2dbeedbce96/core-c2dbeedbce96
- [x] REST commit (38 ms) sha=839257ef61c8f413a1ac241ced33f24fc65b9a69 sequence=1
- [x] REST read (15 ms) path=README.md
- [x] create Git credential (3 ms) scope=write
- [x] Git clone (77 ms) cloned repository
- [x] Git verify REST commit (9 ms) head=839257ef61c8f413a1ac241ced33f24fc65b9a69
- [x] Git commit and push (182 ms) sha=acb11dfcf620a383774b1be42f58d79b6b25f2f2
- [x] REST readback (8 ms) path=result.txt
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

- `git.log` `ece1545f909db2e1ba55930b62100514974e1d3741459e5ceda3ce271294fc7c` (435 bytes)
