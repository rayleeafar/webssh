# make a plan to add below features:
  - 1,node ssh connect method soupport via socks5/http/https proxy or through a jump
  server, jump server also soupport via proxy too
  - 2.add a endpoint to get node system info,backend can get basic info by run some
  shell cmd on remote node server on first add the node then store basic info to
  local db,and update when user click on the sysinfo tab
  - 3.add a batch exec function,user can select nodes to run a cmd and get return
  within a timeout period
  - 4.soupport open a node ssh session in more than one tab,and each tab has its
  own session,and user can switch between tabs to manage different sessions