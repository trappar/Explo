"""Exercise the update helper using temporary Git repositories, without network access."""
import pathlib,subprocess,tempfile
script=str(pathlib.Path(__file__).resolve().with_name('sync-upstream.sh'))
def run(cwd,*args,ok=True):
 p=subprocess.run(args,cwd=cwd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 if ok and p.returncode: raise AssertionError(p.stdout)
 return p
with tempfile.TemporaryDirectory(prefix='explo-sync-') as tmp:
 root=pathlib.Path(tmp);up=root/'upstream';origin=root/'origin.git';fork=root/'fork'
 run(root,'git','init','-q','-b','dev',str(up))
 def identify(repo):
  run(repo,'git','config','user.name','Fixture');run(repo,'git','config','user.email','fixture@example.invalid')
 def commit(repo,name,content,message):
  (repo/name).write_text(content);run(repo,'git','add',name);run(repo,'git','commit','-qm',message)
 identify(up);commit(up,'shared','base\n','base');run(up,'git','tag','v1.0.0')
 run(root,'git','clone','-q','--bare',str(up),str(origin));run(root,'git','clone','-q',str(origin),str(fork));identify(fork)
 run(fork,'git','remote','add','upstream',str(up));commit(fork,'our-fix','keep\n','local fix');run(fork,'git','push','-q','origin','dev')
 commit(up,'official-change','new\n','upstream update');run(up,'git','tag','v1.1.0')
 (fork/'unfinished').write_text('dirty')
 assert run(fork,'sh',script,'v1.1.0',ok=False).returncode!=0
 (fork/'unfinished').unlink()
 p=run(fork,'sh',script,'v1.0.0');assert 'already included' in p.stdout
 p=run(fork,'sh',script,'v1.1.0');assert 'Merge prepared' in p.stdout
 assert (fork/'our-fix').read_text()=='keep\n' and (fork/'official-change').read_text()=='new\n'
 assert run(fork,'git','branch','--show-current').stdout.strip()=='maintenance/upstream-v1.1.0'
 run(fork,'git','rev-parse','--verify','MERGE_HEAD');run(fork,'git','commit','-qm','merge update');run(fork,'git','switch','dev');run(fork,'git','merge','--ff-only','maintenance/upstream-v1.1.0')
 commit(fork,'shared','our behavior\n','fork behavior');commit(up,'shared','official behavior\n','upstream behavior');run(up,'git','tag','v1.2.0')
 p=run(fork,'sh',script,'v1.2.0',ok=False);assert p.returncode!=0 and 'Resolve the merge' in p.stdout
 run(fork,'git','merge','--abort');assert (fork/'shared').read_text()=='our behavior\n';assert (fork/'our-fix').exists()
 print('PASS: dirty-tree refusal, no-op sync, merge preserves fork fix, conflict preserves recoverable state.')
