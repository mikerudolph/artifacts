# Artifacts verification 4c0bbcb6094f

Classification: `product`

Failure: git -c: exit status 128

## Steps

- [x] doctor (4 ms) http://127.0.0.1:57783
- [x] REST create repository (15 ms) repository=agent-4c0bbcb6094f/core-4c0bbcb6094f
- [x] REST commit (46 ms) sha=9c8bb76222678ea611d4e6ec855de9402775561f sequence=1
- [x] REST read (3 ms) path=README.md
- [x] create Git credential (1 ms) scope=write
- [x] Git clone (92 ms) cloned repository
- [x] Git verify REST commit (9 ms) head=9c8bb76222678ea611d4e6ec855de9402775561f
- [ ] Git commit and push (60070 ms) 
- [x] delete repository (28 ms) final server state checked

## Assertions

- [x] repository has Git remote remote present
- [x] REST content matches README.md content matched
- [x] Git sees REST content README matched
- [x] Git sees REST SHA SHA matched
- [x] repository absent after cleanup GET returned not found

## Artifacts

- `git.log` `ef00efd562098c9b1b04d1432760b23af39368f1b9b39459052c42c1d7d7a7a7` (290 bytes)
