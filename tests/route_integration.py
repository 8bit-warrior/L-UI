import http.server, socketserver, threading, tempfile, os, importlib.util
from pathlib import Path
home=tempfile.mkdtemp(prefix='lui-route-int-')
os.environ['LUI_HOME']=home
os.environ['LUI_BIN_DIR']=home+'/bin'
os.environ['LUI_LOG_DIR']=home+'/logs'
os.environ['LUI_XRAY_BIN']=str(Path(__file__).with_name('fake_xray.py'))
spec=importlib.util.spec_from_file_location('lui',Path(__file__).parents[1]/'lui.py')
lui=importlib.util.module_from_spec(spec);spec.loader.exec_module(lui)
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(204);self.end_headers()
    def log_message(self,*a): pass
srv=socketserver.TCPServer(('127.0.0.1',0),H)
threading.Thread(target=srv.serve_forever,daemon=True).start()
st=lui.default_state()
url=f'http://127.0.0.1:{srv.server_address[1]}/'
r=lui.real_route_test(st,url)
print(r)
assert r['success'] is True,r
assert r['http_code']==204,r
assert r['outbound']=='direct',r
assert r['total_ms'] is not None and r['total_ms']>=0,r
srv.shutdown();srv.server_close()
