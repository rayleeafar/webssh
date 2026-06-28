import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  ScrollView,
  ActivityIndicator,
  Alert,
  TextInput,
  Modal,
  Platform,
} from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { WebView } from 'react-native-webview';
import { SafeAreaView } from 'react-native-safe-area-context';
import {
  apiGet,
  apiPost,
  apiDelete,
  getWSTicket,
  getWebSocketBase,
} from '../lib/api';

type TabType = 'terminal' | 'sftp' | 'sysinfo';

interface FileInfo {
  name: string;
  size: number;
  mode: string;
  mod_time: string;
  is_dir: boolean;
}

interface SysInfoData {
  hostname: string;
  os: string;
  kernel: string;
  uptime: string;
  cpu_model: string;
  cpu_cores: number;
  load_avg: string;
  mem_total: number;
  mem_used: number;
  disk_total: string;
  disk_used: string;
  disk_pct: string;
  ip_addr: string;
}

export default function ConsoleScreen() {
  const router = useRouter();
  const params = useLocalSearchParams();
  const nodeId = parseInt(params.nodeId as string);
  const nodeName = params.nodeName as string;

  const [activeTab, setActiveTab] = useState<TabType>('terminal');
  
  // Terminal WebSocket URL state
  const [wsUrl, setWsUrl] = useState<string | null>(null);
  const [terminalLoading, setTerminalLoading] = useState(true);

  // SFTP state
  const [sftpFiles, setSftpFiles] = useState<FileInfo[]>([]);
  const [sftpPath, setSftpPath] = useState('.');
  const [sftpLoading, setSftpLoading] = useState(false);
  const [mkdirVisible, setMkdirVisible] = useState(false);
  const [newDirName, setNewDirName] = useState('');

  // Sysinfo state
  const [sysInfo, setSysInfo] = useState<SysInfoData | null>(null);
  const [sysInfoLoading, setSysInfoLoading] = useState(false);

  // 1. Fetch WebSocket ticket on mount for terminal
  useEffect(() => {
    async function initTerminal() {
      try {
        const ticket = await getWSTicket();
        const base = await getWebSocketBase();
        const url = `${base}/ws/terminal?nodeId=${nodeId}&ticket=${encodeURIComponent(ticket)}`;
        setWsUrl(url);
      } catch (err: any) {
        Alert.alert('Error', err.message || 'Failed to initialize terminal session');
      } finally {
        setTerminalLoading(false);
      }
    }
    if (nodeId) {
      initTerminal();
    }
  }, [nodeId]);

  // 2. Fetch SFTP Files
  const fetchSftpFiles = useCallback(async (path: string) => {
    setSftpLoading(true);
    try {
      const res = await apiGet(`/api/sftp/list?nodeId=${nodeId}&path=${encodeURIComponent(path)}`);
      if (!res.ok) throw new Error('Failed to load files');
      const data = await res.json();
      setSftpFiles(data || []);
    } catch (err: any) {
      Alert.alert('SFTP Error', err.message || 'Failed to list files');
    } finally {
      setSftpLoading(false);
    }
  }, [nodeId]);

  // 3. Fetch Sysinfo
  const fetchSysInfo = useCallback(async () => {
    setSysInfoLoading(true);
    try {
      const res = await apiGet(`/api/nodes/${nodeId}/sysinfo`);
      if (!res.ok) throw new Error('Failed to get system info');
      const data = await res.json();
      setSysInfo(data);
    } catch (err: any) {
      console.log('Sysinfo error:', err);
    } finally {
      setSysInfoLoading(false);
    }
  }, [nodeId]);

  // Handle Tab changes
  useEffect(() => {
    if (activeTab === 'sftp') {
      fetchSftpFiles(sftpPath);
    } else if (activeTab === 'sysinfo') {
      fetchSysInfo();
      const interval = setInterval(fetchSysInfo, 15000);
      return () => clearInterval(interval);
    }
  }, [activeTab, sftpPath, fetchSftpFiles, fetchSysInfo]);

  // SFTP actions
  const handleSftpNavigate = (name: string, isDir: boolean) => {
    if (!isDir) return;
    if (name === '..') {
      const parts = sftpPath.split('/').filter((p) => p && p !== '.');
      parts.pop();
      const newPath = parts.length > 0 ? parts.join('/') : '.';
      setSftpPath(newPath);
      fetchSftpFiles(newPath);
    } else {
      const newPath = sftpPath === '.' ? name : `${sftpPath}/${name}`;
      setSftpPath(newPath);
      fetchSftpFiles(newPath);
    }
  };

  const handleSftpDelete = (name: string) => {
    Alert.alert('Delete File', `Are you sure you want to delete ${name}?`, [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Delete',
        style: 'destructive',
        onPress: async () => {
          try {
            const filePath = sftpPath === '.' ? name : `${sftpPath}/${name}`;
            const res = await apiDelete(`/api/sftp/delete?nodeId=${nodeId}&path=${encodeURIComponent(filePath)}`);
            if (!res.ok) throw new Error('Delete failed');
            fetchSftpFiles(sftpPath);
          } catch (err: any) {
            Alert.alert('Error', err.message || 'Failed to delete');
          }
        },
      },
    ]);
  };

  const handleSftpMkdir = async () => {
    if (!newDirName.trim()) return;
    try {
      const dirPath = sftpPath === '.' ? newDirName.trim() : `${sftpPath}/${newDirName.trim()}`;
      const res = await apiPost('/api/sftp/mkdir', {
        node_id: nodeId,
        path: dirPath,
      });
      if (!res.ok) throw new Error('Failed to create directory');
      setMkdirVisible(false);
      setNewDirName('');
      fetchSftpFiles(sftpPath);
    } catch (err: any) {
      Alert.alert('Error', err.message || 'Failed to create directory');
    }
  };

  // HTML Template for xterm.js WebView
  const getTerminalHtml = () => {
    if (!wsUrl) return '';
    return `
      <!DOCTYPE html>
      <html>
      <head>
        <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no" />
        <link rel="stylesheet" href="https://unpkg.com/xterm@5.3.0/css/xterm.css" />
        <script src="https://unpkg.com/xterm@5.3.0/lib/xterm.js"></script>
        <script src="https://unpkg.com/@xterm/addon-fit@0.11.0/lib/addon-fit.js"></script>
        <style>
          html, body {
            margin: 0; padding: 0; height: 100%; background: #050508; overflow: hidden;
          }
          #terminal {
            width: 100%; height: 100%;
          }
          body::after {
            content: '';
            position: absolute;
            inset: 0;
            background: repeating-linear-gradient(
              0deg,
              transparent,
              transparent 2px,
              rgba(0, 0, 0, 0.04) 2px,
              rgba(0, 0, 0, 0.04) 4px
            );
            pointer-events: none;
            z-index: 10;
          }
          .xterm-viewport {
            overflow-y: auto !important;
          }
        </style>
      </head>
      <body>
        <div id="terminal"></div>
        <script>
          const term = new Terminal({
            cursorBlink: true,
            fontSize: 13,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
            theme: {
              background: '#050508',
              foreground: '#c8d8f0',
              cursor: '#00ffff',
              cursorAccent: '#050508',
              selectionBackground: 'rgba(0,255,255,0.2)',
            },
            scrollback: 5000,
          });
          const fitAddon = new FitAddon.FitAddon();
          term.loadAddon(fitAddon);
          term.open(document.getElementById('terminal'));
          
          // Initial fit
          setTimeout(() => fitAddon.fit(), 200);

          // WebSocket connection
          const ws = new WebSocket("${wsUrl}");
          ws.binaryType = 'arraybuffer';

          ws.onopen = () => {
            term.onData((data) => {
              ws.send(data);
            });
            
            const sendResize = () => {
              fitAddon.fit();
              const dims = fitAddon.proposeDimensions();
              if (dims && ws.readyState === WebSocket.OPEN) {
                const msg = new Uint8Array(5);
                msg[0] = 0;
                msg[1] = (dims.rows >> 8) & 0xff;
                msg[2] = dims.rows & 0xff;
                msg[3] = (dims.cols >> 8) & 0xff;
                msg[4] = dims.cols & 0xff;
                ws.send(msg);
              }
            };
            
            sendResize();
            window.addEventListener('resize', sendResize);
            setInterval(sendResize, 1500); // periodically refit
          };

          ws.onmessage = (event) => {
            if (event.data instanceof ArrayBuffer) {
              term.write(new Uint8Array(event.data));
            } else {
              term.write(event.data);
            }
          };

          ws.onclose = () => {
            term.write('\\r\\n\\r\\n\\x1b[31mConnection closed.\\x1b[0m\\r\\n');
          };

          ws.onerror = () => {
            term.write('\\r\\n\\x1b[31mWebSocket error.\\x1b[0m\\r\\n');
          };
        </script>
      </body>
      </html>
    `;
  };

  const renderProgressBar = (label: string, pct: number, color: string) => {
    const clamped = Math.max(0, Math.min(100, pct));
    const isWarn = clamped > 80;
    const barColor = isWarn ? '#ff3060' : color;
    
    return (
      <View style={styles.progressContainer}>
        <Text style={styles.progressLabel}>{label}</Text>
        <View style={styles.progressBg}>
          <View style={[styles.progressBar, { width: `${clamped}%`, backgroundColor: barColor }]} />
        </View>
        <Text style={[styles.progressVal, isWarn && { color: '#ff3060' }]}>{clamped.toFixed(0)}%</Text>
      </View>
    );
  };

  return (
    <SafeAreaView style={styles.container}>
      {/* Console Header */}
      <View style={styles.header}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.replace('/')}>
          <Text style={{ color: '#00ffff', fontSize: 16 }}>←</Text>
        </TouchableOpacity>
        <View style={{ flex: 1, marginLeft: 12 }}>
          <Text style={styles.headerTitle} numberOfLines={1}>{nodeName.toUpperCase()}</Text>
          <Text style={styles.headerSubtitle}>ACTIVE GATEWAY CONNECTION</Text>
        </View>
        <View style={styles.statusContainer}>
          <View style={styles.statusDot} />
          <Text style={styles.statusText}>ONLINE</Text>
        </View>
      </View>

      {/* Main Content Area */}
      <View style={{ flex: 1, width: '100%' }}>
        {/* Terminal Tab */}
        {activeTab === 'terminal' && (
          <View style={{ flex: 1, backgroundColor: '#050508' }}>
            {terminalLoading ? (
              <View style={styles.centerContainer}>
                <ActivityIndicator size="large" color="#00ffff" />
                <Text style={styles.loadingText}>INITIALIZING SSH OVER WS...</Text>
              </View>
            ) : wsUrl ? (
              <WebView
                originWhitelist={['*']}
                source={{ html: getTerminalHtml() }}
                style={{ flex: 1, backgroundColor: '#050508' }}
                javaScriptEnabled={true}
                domStorageEnabled={true}
              />
            ) : (
              <View style={styles.centerContainer}>
                <Text style={styles.errorText}>FAILED TO ESTABLISH SESSION TICKET</Text>
              </View>
            )}
          </View>
        )}

        {/* SFTP Tab */}
        {activeTab === 'sftp' && (
          <View style={{ flex: 1 }}>
            {/* SFTP Toolbar */}
            <View style={styles.sftpToolbar}>
              <Text style={styles.sftpPath} numberOfLines={1}>
                ~/{sftpPath === '.' ? '' : sftpPath}
              </Text>
              <View style={{ flexDirection: 'row', gap: 8 }}>
                <TouchableOpacity style={styles.sftpBtn} onPress={() => setMkdirVisible(true)}>
                  <Text style={styles.sftpBtnText}>+ FOLDER</Text>
                </TouchableOpacity>
                <TouchableOpacity style={styles.sftpBtn} onPress={() => fetchSftpFiles(sftpPath)}>
                  <Text style={styles.sftpBtnText}>↺</Text>
                </TouchableOpacity>
              </View>
            </View>

            {sftpLoading && sftpFiles.length === 0 ? (
              <View style={styles.centerContainer}>
                <ActivityIndicator size="large" color="#00ffff" />
              </View>
            ) : (
              <ScrollView style={{ flex: 1 }} contentContainerStyle={{ paddingBottom: 16 }}>
                {sftpPath !== '.' && (
                  <TouchableOpacity
                    style={styles.fileRow}
                    onPress={() => handleSftpNavigate('..', true)}
                  >
                    <Text style={{ color: '#00ffff', fontSize: 16, marginRight: 8 }}>▶</Text>
                    <Text style={styles.dirName}>..</Text>
                  </TouchableOpacity>
                )}

                {sftpFiles.map((file) => (
                  <View key={file.name} style={styles.fileRow}>
                    <TouchableOpacity
                      style={{ flex: 1, flexDirection: 'row', alignItems: 'center' }}
                      onPress={() => handleSftpNavigate(file.name, file.is_dir)}
                    >
                      <Text style={{ color: file.is_dir ? '#00ffff' : '#8090b8', fontSize: 16, marginRight: 8 }}>
                        {file.is_dir ? '▶' : '·'}
                      </Text>
                      <Text style={file.is_dir ? styles.dirName : styles.fileName} numberOfLines={1}>
                        {file.name}
                      </Text>
                    </TouchableOpacity>

                    <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
                      {!file.is_dir && (
                        <Text style={styles.fileSize}>
                          {file.size > 1024 * 1024
                            ? `${(file.size / (1024 * 1024)).toFixed(1)} MB`
                            : `${(file.size / 1024).toFixed(1)} KB`}
                        </Text>
                      )}
                      <TouchableOpacity onPress={() => handleSftpDelete(file.name)}>
                        <Text style={{ color: '#ff3060', fontSize: 16, paddingHorizontal: 4 }}>✕</Text>
                      </TouchableOpacity>
                    </View>
                  </View>
                ))}

                {sftpFiles.length === 0 && (
                  <Text style={styles.sftpEmptyText}>EMPTY DIRECTORY</Text>
                )}
              </ScrollView>
            )}
          </View>
        )}

        {/* System Info Tab */}
        {activeTab === 'sysinfo' && (
          <View style={{ flex: 1 }}>
            {sysInfoLoading && !sysInfo ? (
              <View style={styles.centerContainer}>
                <ActivityIndicator size="large" color="#00ffff" />
              </View>
            ) : sysInfo ? (
              <ScrollView style={{ flex: 1 }} contentContainerStyle={{ padding: 16, gap: 16 }}>
                {/* Stats Cards */}
                <View style={styles.sysinfoGrid}>
                  <View style={styles.infoCard}>
                    <Text style={styles.infoCardLabel}>HOSTNAME</Text>
                    <Text style={[styles.infoCardValue, { color: '#00ffff' }]}>{sysInfo.hostname}</Text>
                  </View>
                  <View style={styles.infoCard}>
                    <Text style={styles.infoCardLabel}>IP ADDRESS</Text>
                    <Text style={styles.infoCardValue}>{sysInfo.ip_addr}</Text>
                  </View>
                  <View style={styles.infoCard}>
                    <Text style={styles.infoCardLabel}>OS / KERNEL</Text>
                    <Text style={styles.infoCardValue} numberOfLines={1}>{sysInfo.os} ({sysInfo.kernel})</Text>
                  </View>
                  <View style={styles.infoCard}>
                    <Text style={styles.infoCardLabel}>UPTIME</Text>
                    <Text style={[styles.infoCardValue, { color: '#00ff88' }]}>{sysInfo.uptime}</Text>
                  </View>
                </View>

                {/* Resource Gauges */}
                <View style={styles.sysinfoSection}>
                  <Text style={styles.sectionTitle}>RESOURCE UTILIATION</Text>
                  
                  {/* CPU Progress */}
                  {(() => {
                    const loadVals = sysInfo.load_avg?.split(' ').map(Number) ?? [];
                    const cpuLoad = sysInfo.cpu_cores > 0 && loadVals[0]
                      ? Math.min(100, (loadVals[0] / sysInfo.cpu_cores) * 100)
                      : 0;
                    return renderProgressBar('CPU', cpuLoad, '#00ffff');
                  })()}

                  {/* Memory Progress */}
                  {(() => {
                    const memPct = sysInfo.mem_total > 0 ? (sysInfo.mem_used / sysInfo.mem_total) * 100 : 0;
                    return renderProgressBar('MEM', memPct, '#ff00ff');
                  })()}

                  {/* Disk Progress */}
                  {renderProgressBar('DSK', parseInt(sysInfo.disk_pct || '0'), '#0080ff')}
                </View>

                <View style={styles.sysinfoSection}>
                  <Text style={styles.sectionTitle}>HARDWARE PROFILE</Text>
                  <View style={styles.profileRow}>
                    <Text style={styles.profileLabel}>CPU Model</Text>
                    <Text style={styles.profileValue} numberOfLines={1}>{sysInfo.cpu_model}</Text>
                  </View>
                  <View style={styles.profileRow}>
                    <Text style={styles.profileLabel}>Cores</Text>
                    <Text style={styles.profileValue}>{sysInfo.cpu_cores}</Text>
                  </View>
                  <View style={styles.profileRow}>
                    <Text style={styles.profileLabel}>Total Memory</Text>
                    <Text style={styles.profileValue}>{sysInfo.mem_total} MB</Text>
                  </View>
                  <View style={styles.profileRow}>
                    <Text style={styles.profileLabel}>Total Disk Space</Text>
                    <Text style={styles.profileValue}>{sysInfo.disk_total}</Text>
                  </View>
                </View>
              </ScrollView>
            ) : (
              <View style={styles.centerContainer}>
                <Text style={styles.errorText}>SYSTEM INFORMATION UNAVAILABLE</Text>
              </View>
            )}
          </View>
        )}
      </View>

      {/* Console Tab Selector (Bottom Navigation) */}
      <View style={styles.tabBar}>
        <TouchableOpacity
          style={[styles.tabItem, activeTab === 'terminal' && styles.tabItemActive]}
          onPress={() => setActiveTab('terminal')}
        >
          <Text style={[styles.tabText, activeTab === 'terminal' && styles.tabTextActive]}>TERMINAL</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.tabItem, activeTab === 'sftp' && styles.tabItemActive]}
          onPress={() => setActiveTab('sftp')}
        >
          <Text style={[styles.tabText, activeTab === 'sftp' && styles.tabTextActive]}>SFTP</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.tabItem, activeTab === 'sysinfo' && styles.tabItemActive]}
          onPress={() => setActiveTab('sysinfo')}
        >
          <Text style={[styles.tabText, activeTab === 'sysinfo' && styles.tabTextActive]}>SYSINFO</Text>
        </TouchableOpacity>
      </View>

      {/* Mkdir Modal */}
      <Modal
        animationType="fade"
        transparent={true}
        visible={mkdirVisible}
        onRequestClose={() => setMkdirVisible(false)}
      >
        <View style={styles.alertOverlay}>
          <View style={styles.alertCard}>
            <Text style={styles.alertTitle}>CREATE DIRECTORY</Text>
            <TextInput
              style={styles.alertInput}
              placeholder="Directory name"
              placeholderTextColor="#303060"
              value={newDirName}
              onChangeText={setNewDirName}
              autoFocus
            />
            <View style={styles.alertBtnGroup}>
              <TouchableOpacity style={styles.alertBtn} onPress={() => setMkdirVisible(false)}>
                <Text style={[styles.alertBtnText, { color: '#6070a0' }]}>CANCEL</Text>
              </TouchableOpacity>
              <TouchableOpacity style={[styles.alertBtn, { borderColor: '#00ffff' }]} onPress={handleSftpMkdir}>
                <Text style={[styles.alertBtnText, { color: '#00ffff' }]}>CREATE</Text>
              </TouchableOpacity>
            </View>
          </View>
        </View>
      </Modal>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#050508',
  },
  header: {
    height: 60,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(0, 255, 255, 0.15)',
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    backgroundColor: 'rgba(8, 8, 16, 0.98)',
  },
  backBtn: {
    width: 32,
    height: 32,
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.25)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  headerTitle: {
    fontFamily: 'Orbitron_900Black',
    fontSize: 14,
    color: '#00ffff',
    letterSpacing: 2,
  },
  headerSubtitle: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#6070a0',
    letterSpacing: 1,
  },
  statusContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 136, 0.25)',
    paddingHorizontal: 8,
    paddingVertical: 4,
    backgroundColor: 'rgba(0, 255, 136, 0.03)',
  },
  statusDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    backgroundColor: '#00ff88',
  },
  statusText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#00ff88',
    letterSpacing: 1,
  },
  centerContainer: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
  },
  loadingText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 10,
    color: '#6070a0',
    letterSpacing: 1.5,
  },
  errorText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 11,
    color: '#ff3060',
    letterSpacing: 1.5,
  },
  tabBar: {
    height: 48,
    borderTopWidth: 1,
    borderTopColor: 'rgba(0, 255, 255, 0.15)',
    flexDirection: 'row',
    backgroundColor: 'rgba(8, 8, 16, 0.98)',
  },
  tabItem: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    borderBottomWidth: 2,
    borderBottomColor: 'transparent',
  },
  tabItemActive: {
    borderBottomColor: '#00ffff',
    backgroundColor: 'rgba(0, 255, 255, 0.04)',
  },
  tabText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 10,
    color: '#404070',
    letterSpacing: 1.5,
  },
  tabTextActive: {
    color: '#00ffff',
    textShadowColor: 'rgba(0, 255, 255, 0.5)',
    textShadowOffset: { width: 0, height: 0 },
    textShadowRadius: 6,
  },
  sftpToolbar: {
    height: 40,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(0, 255, 255, 0.08)',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    backgroundColor: '#080810',
  },
  sftpPath: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 13,
    color: '#6070a0',
    flex: 1,
  },
  sftpBtn: {
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.2)',
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  sftpBtnText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#6070a0',
    letterSpacing: 1,
  },
  fileRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(255, 255, 255, 0.02)',
  },
  dirName: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 14,
    color: '#00ffff',
  },
  fileName: {
    fontFamily: 'Rajdhani_500Medium',
    fontSize: 14,
    color: '#8090b8',
  },
  fileSize: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 11,
    color: '#404070',
  },
  sftpEmptyText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 10,
    color: '#252545',
    letterSpacing: 2,
    textAlign: 'center',
    marginTop: 40,
  },
  sysinfoGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  infoCard: {
    flex: 1,
    minWidth: '45%',
    backgroundColor: 'rgba(5, 5, 8, 0.6)',
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.1)',
    paddingVertical: 10,
    paddingHorizontal: 14,
  },
  infoCardLabel: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#404070',
    letterSpacing: 1.5,
    marginBottom: 4,
  },
  infoCardValue: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 14,
    color: '#8090b8',
  },
  sysinfoSection: {
    backgroundColor: 'rgba(5, 5, 8, 0.6)',
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.1)',
    padding: 14,
  },
  sectionTitle: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 9,
    color: '#404070',
    letterSpacing: 2,
    marginBottom: 14,
  },
  progressContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    marginBottom: 12,
  },
  progressLabel: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#404070',
    letterSpacing: 1.5,
    width: 28,
  },
  progressBg: {
    flex: 1,
    height: 6,
    backgroundColor: 'rgba(255, 255, 255, 0.05)',
    borderRadius: 3,
    overflow: 'hidden',
  },
  progressBar: {
    height: '100%',
    borderRadius: 3,
  },
  progressVal: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 11,
    color: '#6070a0',
    width: 32,
    textAlign: 'right',
  },
  profileRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingVertical: 6,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(255,255,255,0.01)',
  },
  profileLabel: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 13,
    color: '#404070',
  },
  profileValue: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 13,
    color: '#8090b8',
    maxWidth: '65%',
  },
  alertOverlay: {
    flex: 1,
    backgroundColor: 'rgba(5, 5, 8, 0.75)',
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 20,
  },
  alertCard: {
    width: '100%',
    maxWidth: 320,
    backgroundColor: 'rgba(13, 13, 26, 0.98)',
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.2)',
    padding: 24,
    borderRadius: 2,
  },
  alertTitle: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 12,
    color: '#00ffff',
    letterSpacing: 1.5,
    marginBottom: 16,
    textAlign: 'center',
  },
  alertInput: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 15,
    color: '#c8d8f0',
    backgroundColor: 'rgba(8, 8, 16, 0.8)',
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(0, 255, 255, 0.3)',
    paddingVertical: 6,
    paddingHorizontal: 4,
    marginBottom: 20,
  },
  alertBtnGroup: {
    flexDirection: 'row',
    gap: 12,
  },
  alertBtn: {
    flex: 1,
    paddingVertical: 10,
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.1)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  alertBtnText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 10,
    letterSpacing: 1.5,
  },
});
