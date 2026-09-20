# Artifacts verification af571c30fbf6

Classification: `none`

## Steps

- [x] doctor (8 ms) http://127.0.0.1:57889
- [x] REST create repository (60 ms) repository=agent-af571c30fbf6/core-af571c30fbf6
- [x] REST commit (335 ms) sha=c15c90b18f339aa8c8164d50f37d388e40ae0442 sequence=1
- [x] REST read (13 ms) path=README.md
- [x] create Git credential (8 ms) scope=write
- [x] Git clone (284 ms) cloned repository
- [x] Git verify REST commit (17 ms) head=c15c90b18f339aa8c8164d50f37d388e40ae0442
- [x] Git commit and push (637 ms) sha=d028e1dbc5950c6e7a09dc9028be1ee1dee7abbc
- [x] REST readback (43 ms) path=result.txt
- [x] refs and WAL visibility (13 ms) refs=2 wal=2
- [x] delete repository (36 ms) final server state checked

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

- `git.log` `f2fc8322247424b4474c365cbe25f1382902a1a2fc1116348991319d75f4beca` (435 bytes)
