import React, { useState, useEffect, useCallback } from 'react';
import {
  View,
  Text,
  TextInput,
  TouchableOpacity,
  FlatList,
  Modal,
  StyleSheet,
  Alert,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  SafeAreaView,
} from 'react-native';
import { useRouter } from 'expo-router';
import {
  initApiConfig,
  setServerUrl,
  getServerUrl,
  setSession,
  clearSession,
  apiGet,
  apiPost,
  apiDelete,
  apiPut,
} from '../lib/api';

interface Node {
  id: number;
  name: string;
  host: string;
  port: number;
  username: string;
  proxy_type?: string;
  proxy_host?: string;
  proxy_port?: number;
  proxy_credential_id?: number;
}

export default function IndexScreen() {
  const router = useRouter();
  
  // App state
  const [loading, setLoading] = useState(true);
  const [isConfigured, setIsConfigured] = useState(false);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  
  // Server URL setup state
  const [serverInput, setServerInput] = useState('');
  
  // Auth state
  const [authMode, setAuthMode] = useState<'login' | 'register'>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [authError, setAuthError] = useState('');
  const [authLoading, setAuthLoading] = useState(false);
  
  // 2FA state
  const [requires2FA, setRequires2FA] = useState(false);
  const [tempToken, setTempToken] = useState('');
  const [totpCode, setTotpCode] = useState('');
  
  // Nodes state
  const [nodes, setNodes] = useState<Node[]>([]);
  const [nodesLoading, setNodesLoading] = useState(false);
  
  // Node Modal state (Add/Edit)
  const [nodeModalVisible, setNodeModalVisible] = useState(false);
  const [editingNode, setEditingNode] = useState<Node | null>(null);
  const [nodeName, setNodeName] = useState('');
  const [nodeHost, setNodeHost] = useState('');
  const [nodePort, setNodePort] = useState('22');
  const [nodeUser, setNodeUser] = useState('');
  const [nodeAuthType, setNodeAuthType] = useState<'password' | 'private_key'>('password');
  const [nodeAuthValue, setNodeAuthValue] = useState('');
  const [nodeModalLoading, setNodeModalLoading] = useState(false);

  // Fetch Nodes from Backend
  const fetchNodes = useCallback(async () => {
    setNodesLoading(true);
    try {
      const res = await apiGet('/api/nodes');
      if (res.status === 401) {
        setIsAuthenticated(false);
        await clearSession();
        return;
      }
      if (!res.ok) throw new Error('Failed to fetch nodes');
      const data = await res.json();
      setNodes(data || []);
    } catch (err) {
      console.log('Fetch nodes error:', err);
    } finally {
      setNodesLoading(false);
    }
  }, []);

  // Initialize App Configuration
  useEffect(() => {
    async function checkSetup() {
      const { serverUrl, token } = await initApiConfig();
      if (serverUrl) {
        setIsConfigured(true);
        setServerInput(serverUrl);
        if (token) {
          setIsAuthenticated(true);
          fetchNodes();
        }
      }
      setLoading(false);
    }
    checkSetup();
  }, [fetchNodes]);

  // Save Server URL
  const handleSaveServer = async () => {
    if (!serverInput.trim()) {
      Alert.alert('Error', 'Please enter a valid server URL');
      return;
    }
    await setServerUrl(serverInput.trim());
    setIsConfigured(true);
  };

  // Handle Login or Registration
  const handleAuthSubmit = async () => {
    setAuthError('');
    if (!username.trim() || !password.trim()) {
      setAuthError('Username and password are required');
      return;
    }
    
    setAuthLoading(true);
    try {
      if (authMode === 'login') {
        const res = await apiPost('/api/auth/login', { username: username.trim(), password });
        if (!res.ok) {
          throw new Error('Invalid username or password');
        }
        const data = await res.json();
        if (data.requires_2fa) {
          setTempToken(data.temp_token);
          setRequires2FA(true);
          setAuthLoading(false);
          return;
        }
        await setSession(data.token, data.csrf_token);
        setIsAuthenticated(true);
        fetchNodes();
      } else {
        const res = await apiPost('/api/auth/register', { username: username.trim(), password });
        if (!res.ok) {
          const text = await res.text();
          throw new Error(text || 'Registration failed');
        }
        Alert.alert('Success', 'Operator registered successfully. Please sign in.');
        setAuthMode('login');
        setPassword('');
      }
    } catch (err: any) {
      setAuthError(err.message || 'Authentication failed');
    } finally {
      setAuthLoading(false);
    }
  };

  // Verify 2FA TOTP Code
  const handleVerify2FA = async () => {
    setAuthError('');
    if (totpCode.length !== 6) {
      setAuthError('Please enter a 6-digit code');
      return;
    }
    
    setAuthLoading(true);
    try {
      const res = await apiPost('/api/auth/2fa/verify', {
        temp_token: tempToken,
        code: totpCode,
      });
      if (!res.ok) {
        throw new Error('Invalid authenticator code');
      }
      const data = await res.json();
      await setSession(data.token, data.csrf_token);
      setRequires2FA(false);
      setTotpCode('');
      setIsAuthenticated(true);
      fetchNodes();
    } catch (err: any) {
      setAuthError(err.message || '2FA verification failed');
    } finally {
      setAuthLoading(false);
    }
  };

  // Save Node (Create or Update)
  const handleSaveNode = async () => {
    if (!nodeName.trim() || !nodeHost.trim() || !nodeUser.trim()) {
      Alert.alert('Error', 'Name, Host and Username are required');
      return;
    }
    
    setNodeModalLoading(true);
    try {
      const payload = {
        name: nodeName.trim(),
        host: nodeHost.trim(),
        port: parseInt(nodePort.trim()) || 22,
        username: nodeUser.trim(),
        auth_type: nodeAuthType,
        credentials: nodeAuthValue,
      };
      
      let res;
      if (editingNode) {
        res = await apiPut(`/api/nodes/${editingNode.id}`, payload);
      } else {
        res = await apiPost('/api/nodes', payload);
      }
      
      if (!res.ok) {
        const txt = await res.text();
        throw new Error(txt || 'Failed to save node');
      }
      
      setNodeModalVisible(false);
      resetNodeForm();
      fetchNodes();
    } catch (err: any) {
      Alert.alert('Error', err.message || 'Failed to save node');
    } finally {
      setNodeModalLoading(false);
    }
  };

  // Delete Node
  const handleDeleteNode = (id: number) => {
    Alert.alert(
      'Delete Node',
      'Are you sure you want to delete this node?',
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Delete',
          style: 'destructive',
          onPress: async () => {
            try {
              const res = await apiDelete(`/api/nodes/${id}`);
              if (!res.ok) throw new Error('Failed to delete node');
              fetchNodes();
            } catch (err: any) {
              Alert.alert('Error', err.message || 'Delete failed');
            }
          },
        },
      ]
    );
  };

  const resetNodeForm = () => {
    setEditingNode(null);
    setNodeName('');
    setNodeHost('');
    setNodePort('22');
    setNodeUser('');
    setNodeAuthType('password');
    setNodeAuthValue('');
  };

  const openEditNode = (node: Node) => {
    setEditingNode(node);
    setNodeName(node.name);
    setNodeHost(node.host);
    setNodePort(String(node.port));
    setNodeUser(node.username);
    setNodeAuthType('password'); // Credentials are encrypted and not returned in GET
    setNodeAuthValue('');
    setNodeModalVisible(true);
  };

  const handleLogout = async () => {
    try {
      await apiPost('/api/auth/logout');
    } catch {
      // ignore
    }
    await clearSession();
    setIsAuthenticated(false);
    setNodes([]);
  };

  // Loading Screen
  if (loading) {
    return (
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color="#00ffff" />
      </View>
    );
  }

  // 1. Server Configuration Screen
  if (!isConfigured) {
    return (
      <SafeAreaView style={styles.container}>
        <KeyboardAvoidingView
          behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
          style={styles.centerView}
        >
          <View style={styles.authCard}>
            <Text style={styles.title}>◈ WEBSSH</Text>
            <Text style={styles.subtitle}>SERVER GATEWAY SETUP</Text>
            
            <View style={styles.inputContainer}>
              <Text style={styles.label}>BACKEND SERVER URL</Text>
              <TextInput
                style={styles.input}
                placeholder="e.g. http://192.168.1.10:8080"
                placeholderTextColor="#303060"
                value={serverInput}
                onChangeText={setServerInput}
                autoCapitalize="none"
                autoCorrect={false}
              />
            </View>
            
            <TouchableOpacity style={styles.button} onPress={handleSaveServer}>
              <Text style={styles.buttonText}>CONNECT GATEWAY</Text>
            </TouchableOpacity>
          </View>
        </KeyboardAvoidingView>
      </SafeAreaView>
    );
  }

  // 2. Login / Register / 2FA Screen
  if (!isAuthenticated) {
    return (
      <SafeAreaView style={styles.container}>
        <KeyboardAvoidingView
          behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
          style={styles.centerView}
        >
          <View style={styles.authCard}>
            <Text style={styles.title}>◈ WEBSSH</Text>
            <Text style={styles.subtitle}>
              {requires2FA ? 'TWO-FACTOR AUTHENTICATION' : authMode === 'login' ? 'SECURE TERMINAL ACCESS' : 'NEW OPERATOR REGISTRATION'}
            </Text>
            
            {authError ? <Text style={styles.errorText}>{authError}</Text> : null}
            
            {requires2FA ? (
              // 2FA TOTP Code Entry
              <View style={{ width: '100%' }}>
                <Text style={styles.infoText}>Enter the 6-digit code from your authenticator app</Text>
                <View style={styles.inputContainer}>
                  <Text style={styles.label}>AUTHENTICATOR CODE</Text>
                  <TextInput
                    style={[styles.input, { textAlign: 'center', letterSpacing: 8, fontSize: 18 }]}
                    placeholder="000000"
                    placeholderTextColor="#303060"
                    value={totpCode}
                    onChangeText={(txt) => setTotpCode(txt.replace(/\D/g, '').substring(0, 6))}
                    keyboardType="numeric"
                    maxLength={6}
                  />
                </View>
                
                <TouchableOpacity 
                  style={styles.button} 
                  onPress={handleVerify2FA}
                  disabled={authLoading}
                >
                  {authLoading ? (
                    <ActivityIndicator size="small" color="#00ffff" />
                  ) : (
                    <Text style={styles.buttonText}>VERIFY CODE</Text>
                  )}
                </TouchableOpacity>

                <TouchableOpacity 
                  style={styles.textButton} 
                  onPress={() => { setRequires2FA(false); setTotpCode(''); }}
                >
                  <Text style={styles.textButtonText}>← BACK TO LOGIN</Text>
                </TouchableOpacity>
              </View>
            ) : (
              // Username / Password Entry
              <View style={{ width: '100%' }}>
                <View style={styles.inputContainer}>
                  <Text style={styles.label}>USERNAME</Text>
                  <TextInput
                    style={styles.input}
                    placeholder="Operator name"
                    placeholderTextColor="#303060"
                    value={username}
                    onChangeText={setUsername}
                    autoCapitalize="none"
                    autoCorrect={false}
                  />
                </View>
                
                <View style={styles.inputContainer}>
                  <Text style={styles.label}>PASSWORD</Text>
                  <TextInput
                    style={styles.input}
                    placeholder="Security passcode"
                    placeholderTextColor="#303060"
                    value={password}
                    onChangeText={setPassword}
                    secureTextEntry
                    autoCapitalize="none"
                    autoCorrect={false}
                  />
                </View>
                
                <TouchableOpacity 
                  style={styles.button} 
                  onPress={handleAuthSubmit}
                  disabled={authLoading}
                >
                  {authLoading ? (
                    <ActivityIndicator size="small" color="#00ffff" />
                  ) : (
                    <Text style={styles.buttonText}>
                      {authMode === 'login' ? 'INITIATE SESSION' : 'REGISTER OPERATOR'}
                    </Text>
                  )}
                </TouchableOpacity>
                
                <TouchableOpacity
                  style={styles.textButton}
                  onPress={() => {
                    setAuthMode(authMode === 'login' ? 'register' : 'login');
                    setAuthError('');
                  }}
                >
                  <Text style={styles.textButtonText}>
                    {authMode === 'login' ? 'No account? REGISTER' : 'Already registered? SIGN IN'}
                  </Text>
                </TouchableOpacity>

                <TouchableOpacity
                  style={[styles.textButton, { marginTop: 10 }]}
                  onPress={() => setIsConfigured(false)}
                >
                  <Text style={[styles.textButtonText, { color: '#6070a0' }]}>CHANGE SERVER URL</Text>
                </TouchableOpacity>
              </View>
            )}
          </View>
        </KeyboardAvoidingView>
      </SafeAreaView>
    );
  }

  // 3. Main Nodes Dashboard Screen
  return (
    <SafeAreaView style={[styles.container, { paddingBottom: 0 }]}>
      {/* Header */}
      <View style={styles.header}>
        <View>
          <Text style={styles.headerTitle}>◈ WEBSSH</Text>
          <Text style={styles.headerSubtitle}>NODES CONSOLE</Text>
        </View>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
          <TouchableOpacity 
            style={styles.headerIconBtn} 
            onPress={() => router.push('/settings')}
          >
            <Text style={{ color: '#00ffff', fontSize: 16 }}>⚙</Text>
          </TouchableOpacity>
          <TouchableOpacity 
            style={[styles.headerIconBtn, { borderColor: 'rgba(255,48,96,0.3)' }]} 
            onPress={handleLogout}
          >
            <Text style={{ color: '#ff3060', fontSize: 14 }}>⏻</Text>
          </TouchableOpacity>
        </View>
      </View>

      {/* Nodes List */}
      <View style={{ flex: 1, width: '100%', paddingHorizontal: 16 }}>
        {nodesLoading && nodes.length === 0 ? (
          <View style={styles.centerContainer}>
            <ActivityIndicator size="large" color="#00ffff" />
          </View>
        ) : nodes.length === 0 ? (
          <View style={styles.centerContainer}>
            <Text style={styles.emptyText}>NO ACTIVE NODES CONFIGURED</Text>
            <TouchableOpacity 
              style={[styles.button, { width: 200, marginTop: 20 }]}
              onPress={() => { resetNodeForm(); setNodeModalVisible(true); }}
            >
              <Text style={styles.buttonText}>+ ADD FIRST NODE</Text>
            </TouchableOpacity>
          </View>
        ) : (
          <FlatList
            data={nodes}
            keyExtractor={(item) => String(item.id)}
            contentContainerStyle={{ paddingVertical: 12, gap: 10 }}
            refreshing={nodesLoading}
            onRefresh={fetchNodes}
            renderItem={({ item }) => (
              <TouchableOpacity
                style={styles.nodeCard}
                onPress={() => router.push(`/console?nodeId=${item.id}&nodeName=${encodeURIComponent(item.name)}`)}
              >
                <View style={{ flex: 1 }}>
                  <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                    <View style={styles.statusDot} />
                    <Text style={styles.nodeName}>{item.name}</Text>
                  </View>
                  <Text style={styles.nodeDetail}>
                    {item.username}@{item.host}:{item.port}
                  </Text>
                </View>
                <View style={{ flexDirection: 'row', gap: 8 }}>
                  <TouchableOpacity 
                    style={styles.actionBtn} 
                    onPress={() => openEditNode(item)}
                  >
                    <Text style={{ color: '#0080ff', fontSize: 12 }}>✎</Text>
                  </TouchableOpacity>
                  <TouchableOpacity 
                    style={[styles.actionBtn, { borderColor: 'rgba(255,48,96,0.3)' }]} 
                    onPress={() => handleDeleteNode(item.id)}
                  >
                    <Text style={{ color: '#ff3060', fontSize: 12 }}>✕</Text>
                  </TouchableOpacity>
                </View>
              </TouchableOpacity>
            )}
          />
        )}
      </View>

      {/* Floating Add Node Button */}
      {nodes.length > 0 && (
        <TouchableOpacity 
          style={styles.fab}
          onPress={() => { resetNodeForm(); setNodeModalVisible(true); }}
        >
          <Text style={styles.fabText}>+</Text>
        </TouchableOpacity>
      )}

      {/* Add / Edit Node Modal */}
      <Modal
        animationType="slide"
        transparent={true}
        visible={nodeModalVisible}
        onRequestClose={() => setNodeModalVisible(false)}
      >
        <View style={styles.modalOverlay}>
          <View style={styles.modalContent}>
            <View style={styles.modalHeader}>
              <Text style={styles.modalTitle}>{editingNode ? 'EDIT NODE' : 'ADD NEW NODE'}</Text>
              <TouchableOpacity onPress={() => setNodeModalVisible(false)}>
                <Text style={{ color: '#6070a0', fontSize: 20 }}>✕</Text>
              </TouchableOpacity>
            </View>
            
            <ScrollView style={{ flex: 1 }} contentContainerStyle={{ paddingVertical: 16, gap: 16 }}>
              <View style={styles.inputContainer}>
                <Text style={styles.label}>NODE NAME</Text>
                <TextInput
                  style={styles.input}
                  placeholder="e.g. Production Web"
                  placeholderTextColor="#303060"
                  value={nodeName}
                  onChangeText={setNodeName}
                />
              </View>

              <View style={{ flexDirection: 'row', gap: 12 }}>
                <View style={[styles.inputContainer, { flex: 3 }]}>
                  <Text style={styles.label}>HOST / IP ADDRESS</Text>
                  <TextInput
                    style={styles.input}
                    placeholder="e.g. 192.168.1.100"
                    placeholderTextColor="#303060"
                    value={nodeHost}
                    onChangeText={setNodeHost}
                    autoCapitalize="none"
                    autoCorrect={false}
                  />
                </View>
                <View style={[styles.inputContainer, { flex: 1 }]}>
                  <Text style={styles.label}>PORT</Text>
                  <TextInput
                    style={styles.input}
                    placeholder="22"
                    placeholderTextColor="#303060"
                    value={nodePort}
                    onChangeText={setNodePort}
                    keyboardType="numeric"
                  />
                </View>
              </View>

              <View style={styles.inputContainer}>
                <Text style={styles.label}>SSH USERNAME</Text>
                <TextInput
                  style={styles.input}
                  placeholder="root"
                  placeholderTextColor="#303060"
                  value={nodeUser}
                  onChangeText={setNodeUser}
                  autoCapitalize="none"
                  autoCorrect={false}
                />
              </View>

              <View style={styles.inputContainer}>
                <Text style={styles.label}>AUTHENTICATION METHOD</Text>
                <View style={{ flexDirection: 'row', gap: 12, marginTop: 6 }}>
                  <TouchableOpacity 
                    style={[styles.tabBtn, nodeAuthType === 'password' && styles.tabBtnActive]}
                    onPress={() => setNodeAuthType('password')}
                  >
                    <Text style={[styles.tabBtnText, nodeAuthType === 'password' && styles.tabBtnTextActive]}>PASSWORD</Text>
                  </TouchableOpacity>
                  <TouchableOpacity 
                    style={[styles.tabBtn, nodeAuthType === 'private_key' && styles.tabBtnActive]}
                    onPress={() => setNodeAuthType('private_key')}
                  >
                    <Text style={[styles.tabBtnText, nodeAuthType === 'private_key' && styles.tabBtnTextActive]}>PRIVATE KEY</Text>
                  </TouchableOpacity>
                </View>
              </View>

              <View style={styles.inputContainer}>
                <Text style={styles.label}>
                  {nodeAuthType === 'password' ? 'SSH PASSWORD' : 'PRIVATE KEY PEM VALUE'}
                </Text>
                <TextInput
                  style={[styles.input, nodeAuthType === 'private_key' && { height: 100, textAlignVertical: 'top' }]}
                  placeholder={nodeAuthType === 'password' ? 'Leave empty if unchanged' : '-----BEGIN OPENSSH PRIVATE KEY-----'}
                  placeholderTextColor="#303060"
                  value={nodeAuthValue}
                  onChangeText={setNodeAuthValue}
                  secureTextEntry={nodeAuthType === 'password'}
                  multiline={nodeAuthType === 'private_key'}
                  autoCapitalize="none"
                  autoCorrect={false}
                />
              </View>
              
              <TouchableOpacity 
                style={[styles.button, { marginTop: 8 }]} 
                onPress={handleSaveNode}
                disabled={nodeModalLoading}
              >
                {nodeModalLoading ? (
                  <ActivityIndicator size="small" color="#00ffff" />
                ) : (
                  <Text style={styles.buttonText}>{editingNode ? 'UPDATE CONNECTION' : 'ESTABLISH CONNECTION'}</Text>
                )}
              </TouchableOpacity>
            </ScrollView>
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
  loadingContainer: {
    flex: 1,
    backgroundColor: '#050508',
    alignItems: 'center',
    justifyContent: 'center',
  },
  centerView: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 20,
  },
  authCard: {
    width: '100%',
    maxWidth: 400,
    backgroundColor: 'rgba(13, 13, 26, 0.95)',
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.2)',
    padding: 32,
    borderRadius: 2,
    alignItems: 'center',
  },
  title: {
    fontFamily: 'Orbitron_900Black',
    fontSize: 26,
    color: '#00ffff',
    letterSpacing: 4,
    marginBottom: 6,
    textShadowColor: 'rgba(0, 255, 255, 0.6)',
    textShadowOffset: { width: 0, height: 0 },
    textShadowRadius: 10,
  },
  subtitle: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 10,
    color: '#6070a0',
    letterSpacing: 2,
    marginBottom: 28,
  },
  inputContainer: {
    width: '100%',
    marginBottom: 20,
  },
  label: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 9,
    color: '#6070a0',
    letterSpacing: 1.5,
    marginBottom: 6,
  },
  input: {
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 15,
    color: '#c8d8f0',
    backgroundColor: 'rgba(8, 8, 16, 0.8)',
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(0, 255, 255, 0.3)',
    paddingVertical: 8,
    paddingHorizontal: 4,
  },
  button: {
    width: '100%',
    borderWidth: 1,
    borderColor: '#00ffff',
    paddingVertical: 12,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: 'rgba(0, 255, 255, 0.04)',
    shadowColor: '#00ffff',
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.25,
    shadowRadius: 6,
    marginTop: 10,
  },
  buttonText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 11,
    color: '#00ffff',
    letterSpacing: 2,
  },
  textButton: {
    marginTop: 20,
  },
  textButtonText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 10,
    color: '#00ffff',
    letterSpacing: 1,
  },
  errorText: {
    color: '#ff3060',
    fontFamily: 'Rajdhani_600SemiBold',
    fontSize: 13,
    marginBottom: 16,
    textAlign: 'center',
  },
  infoText: {
    color: '#6070a0',
    fontFamily: 'Rajdhani_500Medium',
    fontSize: 13,
    textAlign: 'center',
    marginBottom: 16,
    lineHeight: 18,
  },
  header: {
    height: 60,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(0, 255, 255, 0.15)',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 20,
    backgroundColor: 'rgba(8, 8, 16, 0.98)',
  },
  headerTitle: {
    fontFamily: 'Orbitron_900Black',
    fontSize: 16,
    color: '#00ffff',
    letterSpacing: 2,
    textShadowColor: 'rgba(0, 255, 255, 0.6)',
    textShadowOffset: { width: 0, height: 0 },
    textShadowRadius: 8,
  },
  headerSubtitle: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#6070a0',
    letterSpacing: 1.5,
  },
  headerIconBtn: {
    width: 32,
    height: 32,
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.25)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  centerContainer: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  emptyText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 11,
    color: '#303060',
    letterSpacing: 2,
  },
  nodeCard: {
    backgroundColor: '#0a0a14',
    borderLeftWidth: 2,
    borderLeftColor: '#00ffff',
    paddingVertical: 12,
    paddingHorizontal: 16,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.05)',
  },
  statusDot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    backgroundColor: '#00ff88',
    shadowColor: '#00ff88',
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.8,
    shadowRadius: 4,
  },
  nodeName: {
    fontFamily: 'Rajdhani_700Bold',
    fontSize: 15,
    color: '#c8d8f0',
    letterSpacing: 0.5,
  },
  nodeDetail: {
    fontFamily: 'Rajdhani_500Medium',
    fontSize: 12,
    color: '#6070a0',
    paddingLeft: 14,
  },
  actionBtn: {
    width: 26,
    height: 26,
    borderWidth: 1,
    borderColor: 'rgba(0, 128, 255, 0.3)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  fab: {
    position: 'absolute',
    bottom: 24,
    right: 24,
    width: 50,
    height: 50,
    borderRadius: 25,
    backgroundColor: '#050508',
    borderWidth: 1,
    borderColor: '#00ffff',
    alignItems: 'center',
    justifyContent: 'center',
    shadowColor: '#00ffff',
    shadowOffset: { width: 0, height: 0 },
    shadowOpacity: 0.4,
    shadowRadius: 8,
    elevation: 5,
  },
  fabText: {
    fontSize: 24,
    color: '#00ffff',
    lineHeight: 28,
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: 'rgba(5, 5, 8, 0.85)',
    justifyContent: 'flex-end',
  },
  modalContent: {
    height: '80%',
    backgroundColor: 'rgba(13, 13, 26, 0.98)',
    borderTopWidth: 1,
    borderTopColor: 'rgba(0, 255, 255, 0.2)',
    paddingHorizontal: 20,
    paddingTop: 20,
  },
  modalHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingBottom: 12,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(0, 255, 255, 0.1)',
  },
  modalTitle: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 14,
    color: '#00ffff',
    letterSpacing: 2,
  },
  tabBtn: {
    flex: 1,
    paddingVertical: 8,
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.2)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  tabBtnActive: {
    borderColor: '#00ffff',
    backgroundColor: 'rgba(0, 255, 255, 0.08)',
  },
  tabBtnText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 9,
    color: '#6070a0',
    letterSpacing: 1,
  },
  tabBtnTextActive: {
    color: '#00ffff',
  },
});
