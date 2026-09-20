# Artifacts verification cff2a1728e6e

Classification: `none`

## Steps

- [x] doctor (4 ms) http://127.0.0.1:50715
- [x] REST create repository (12 ms) repository=agent-cff2a1728e6e/core-cff2a1728e6e
- [x] REST commit (30 ms) sha=0fb45c793c049829904a9d8f77aba1e6d5ce746b sequence=1
- [x] REST read (2 ms) path=README.md
- [x] create Git credential (0 ms) scope=write
- [x] Git clone (60 ms) cloned repository
- [x] Git verify REST commit (10 ms) head=0fb45c793c049829904a9d8f77aba1e6d5ce746b
- [x] Git commit and push (171 ms) sha=3055cc10f43ea58141de77c68760a48a06bab76b
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

- `git.log` `1dd43982c5715daa0b9d07361a996b65530c583afa4d8907b59b6c1179316355` (433 bytes)
