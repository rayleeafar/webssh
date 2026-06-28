import React, { useState, useEffect } from 'react';
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  TextInput,
  ActivityIndicator,
  Alert,
  ScrollView,
} from 'react-native';
import { useRouter } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import {
  apiGet,
  apiPost,
  getServerUrl,
  setServerUrl,
} from '../lib/api';

type FlowState = 'idle' | 'setup' | 'disabling';

export default function SettingsScreen() {
  const router = useRouter();

  const [username, setUsername] = useState('');
  const [serverUrl, setServerUrlState] = useState('');
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [flowState, setFlowState] = useState<FlowState>('idle');
  const [setupSecret, setSetupSecret] = useState('');
  const [code, setCode] = useState('');
  const [loading, setLoading] = useState(false);
  const [pageLoading, setPageLoading] = useState(true);

  // Initialize and load settings
  useEffect(() => {
    async function loadSettings() {
      try {
        const sUrl = await getServerUrl();
        setServerUrlState(sUrl || '');

        // Fetch user profile
        const meRes = await apiGet('/api/auth/me');
        if (meRes.ok) {
          const me = await meRes.json();
          setUsername(me.username);
        }

        // Fetch 2FA status
        const statusRes = await apiGet('/api/auth/2fa/status');
        if (statusRes.ok) {
          const st = await statusRes.json();
          setTotpEnabled(st.enabled);
        }
      } catch (err) {
        console.log('Load settings error:', err);
      } finally {
        setPageLoading(false);
      }
    }
    loadSettings();
  }, []);

  const handleUpdateServerUrl = async () => {
    if (!serverUrl.trim()) {
      Alert.alert('Error', 'Server URL cannot be empty');
      return;
    }
    await setServerUrl(serverUrl.trim());
    Alert.alert('Success', 'Server URL updated successfully');
  };

  const handleStartSetup = async () => {
    setLoading(true);
    try {
      const res = await apiGet('/api/auth/2fa/setup');
      if (!res.ok) throw new Error('Failed to generate 2FA setup');
      const data = await res.json();
      setSetupSecret(data.secret);
      setFlowState('setup');
      setCode('');
    } catch (err: any) {
      Alert.alert('Error', err.message || 'Failed to start 2FA setup');
    } finally {
      setLoading(false);
    }
  };

  const handleConfirmEnable = async () => {
    if (code.length !== 6) {
      Alert.alert('Error', 'Please enter a 6-digit code');
      return;
    }
    setLoading(true);
    try {
      const res = await apiPost('/api/auth/2fa/enable', {
        secret: setupSecret,
        code,
      });
      if (!res.ok) throw new Error('Invalid verification code');
      setTotpEnabled(true);
      setFlowState('idle');
      setCode('');
      Alert.alert('Success', 'Two-factor authentication enabled successfully.');
    } catch (err: any) {
      Alert.alert('Error', err.message || 'Failed to enable 2FA');
    } finally {
      setLoading(false);
    }
  };

  const handleConfirmDisable = async () => {
    if (code.length !== 6) {
      Alert.alert('Error', 'Please enter a 6-digit code');
      return;
    }
    setLoading(true);
    try {
      const res = await apiPost('/api/auth/2fa/disable', { code });
      if (!res.ok) throw new Error('Invalid verification code');
      setTotpEnabled(false);
      setFlowState('idle');
      setCode('');
      Alert.alert('Success', 'Two-factor authentication disabled.');
    } catch (err: any) {
      Alert.alert('Error', err.message || 'Failed to disable 2FA');
    } finally {
      setLoading(false);
    }
  };

  if (pageLoading) {
    return (
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color="#00ffff" />
      </View>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity style={styles.backBtn} onPress={() => router.replace('/')}>
          <Text style={{ color: '#00ffff', fontSize: 16 }}>←</Text>
        </TouchableOpacity>
        <View style={{ flex: 1, marginLeft: 12 }}>
          <Text style={styles.headerTitle}>⚙ SETTINGS</Text>
          <Text style={styles.headerSubtitle}>SECURITY & CONFIGURATION</Text>
        </View>
      </View>

      <ScrollView style={{ flex: 1 }} contentContainerStyle={{ padding: 20, gap: 24 }}>
        {/* Profile Card */}
        <View style={styles.card}>
          <Text style={styles.cardTitle}>OPERATOR PROFILE</Text>
          <View style={styles.profileRow}>
            <Text style={styles.profileLabel}>USERNAME</Text>
            <Text style={styles.profileValue}>{username.toUpperCase()}</Text>
          </View>
        </View>

        {/* Server Config Card */}
        <View style={styles.card}>
          <Text style={styles.cardTitle}>GATEWAY CONFIGURATION</Text>
          <View style={styles.inputContainer}>
            <Text style={styles.label}>SERVER ENDPOINT URL</Text>
            <TextInput
              style={styles.input}
              placeholder="http://192.168.1.10:8080"
              placeholderTextColor="#303060"
              value={serverUrl}
              onChangeText={setServerUrlState}
              autoCapitalize="none"
              autoCorrect={false}
            />
          </View>
          <TouchableOpacity style={styles.button} onPress={handleUpdateServerUrl}>
            <Text style={styles.buttonText}>UPDATE ENDPOINT</Text>
          </TouchableOpacity>
        </View>

        {/* 2FA Security Card */}
        <View style={styles.card}>
          <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
            <Text style={styles.cardTitle}>TWO-FACTOR AUTHENTICATION</Text>
            <View style={[styles.badge, totpEnabled ? styles.badgeActive : styles.badgeInactive]}>
              <Text style={[styles.badgeText, totpEnabled ? styles.badgeTextActive : styles.badgeTextInactive]}>
                {totpEnabled ? 'ENABLED' : 'DISABLED'}
              </Text>
            </View>
          </View>

          {flowState === 'idle' && (
            <View>
              <Text style={styles.infoText}>
                Protect your operator session with a TOTP authenticator app (Google Authenticator, Authy, etc.).
              </Text>
              {!totpEnabled ? (
                <TouchableOpacity style={styles.button} onPress={handleStartSetup}>
                  <Text style={styles.buttonText}>ENABLE 2FA</Text>
                </TouchableOpacity>
              ) : (
                <TouchableOpacity style={[styles.button, { borderColor: '#ff3060' }]} onPress={() => setFlowState('disabling')}>
                  <Text style={[styles.buttonText, { color: '#ff3060' }]}>DISABLE 2FA</Text>
                </TouchableOpacity>
              )}
            </View>
          )}

          {flowState === 'setup' && (
            <View style={{ gap: 16 }}>
              <Text style={styles.infoText}>
                Copy the secret key below and add it manually to your authenticator app, then enter the 6-digit code.
              </Text>

              <View style={styles.secretBox}>
                <Text style={styles.secretLabel}>MANUAL ENTRY KEY</Text>
                <Text style={styles.secretText} selectable={true}>{setupSecret}</Text>
              </View>

              <View style={styles.inputContainer}>
                <Text style={styles.label}>ENTER VERIFICATION CODE</Text>
                <TextInput
                  style={[styles.input, { textAlign: 'center', fontSize: 18, letterSpacing: 6 }]}
                  placeholder="000000"
                  placeholderTextColor="#303060"
                  value={code}
                  onChangeText={(txt) => setCode(txt.replace(/\D/g, '').substring(0, 6))}
                  keyboardType="numeric"
                  maxLength={6}
                />
              </View>

              <View style={{ flexDirection: 'row', gap: 12 }}>
                <TouchableOpacity style={[styles.button, { flex: 1 }]} onPress={handleConfirmEnable} disabled={loading}>
                  {loading ? <ActivityIndicator size="small" color="#00ffff" /> : <Text style={styles.buttonText}>CONFIRM</Text>}
                </TouchableOpacity>
                <TouchableOpacity style={[styles.button, { flex: 1, borderColor: '#6070a0' }]} onPress={() => setFlowState('idle')}>
                  <Text style={[styles.buttonText, { color: '#6070a0' }]}>CANCEL</Text>
                </TouchableOpacity>
              </View>
            </View>
          )}

          {flowState === 'disabling' && (
            <View style={{ gap: 16 }}>
              <Text style={styles.infoText}>
                Enter your 6-digit authenticator code to disable two-factor authentication.
              </Text>

              <View style={styles.inputContainer}>
                <Text style={styles.label}>ENTER VERIFICATION CODE</Text>
                <TextInput
                  style={[styles.input, { textAlign: 'center', fontSize: 18, letterSpacing: 6 }]}
                  placeholder="000000"
                  placeholderTextColor="#303060"
                  value={code}
                  onChangeText={(txt) => setCode(txt.replace(/\D/g, '').substring(0, 6))}
                  keyboardType="numeric"
                  maxLength={6}
                />
              </View>

              <View style={{ flexDirection: 'row', gap: 12 }}>
                <TouchableOpacity style={[styles.button, { flex: 1, borderColor: '#ff3060' }]} onPress={handleConfirmDisable} disabled={loading}>
                  {loading ? <ActivityIndicator size="small" color="#ff3060" /> : <Text style={[styles.buttonText, { color: '#ff3060' }]}>DISABLE</Text>}
                </TouchableOpacity>
                <TouchableOpacity style={[styles.button, { flex: 1, borderColor: '#6070a0' }]} onPress={() => setFlowState('idle')}>
                  <Text style={[styles.buttonText, { color: '#6070a0' }]}>CANCEL</Text>
                </TouchableOpacity>
              </View>
            </View>
          )}
        </View>
      </ScrollView>
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
  card: {
    backgroundColor: 'rgba(13, 13, 26, 0.95)',
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.15)',
    padding: 20,
    borderRadius: 2,
  },
  cardTitle: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 10,
    color: '#00ffff',
    letterSpacing: 1.5,
    marginBottom: 16,
  },
  profileRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  profileLabel: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 9,
    color: '#404070',
    letterSpacing: 1,
  },
  profileValue: {
    fontFamily: 'Rajdhani_700Bold',
    fontSize: 15,
    color: '#c8d8f0',
  },
  inputContainer: {
    width: '100%',
    marginBottom: 16,
  },
  label: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#6070a0',
    letterSpacing: 1,
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
  },
  buttonText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 11,
    color: '#00ffff',
    letterSpacing: 2,
  },
  badge: {
    borderWidth: 1,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  badgeActive: {
    borderColor: 'rgba(0, 255, 128, 0.5)',
    backgroundColor: 'rgba(0, 255, 128, 0.04)',
  },
  badgeInactive: {
    borderColor: 'rgba(96, 112, 160, 0.3)',
    backgroundColor: 'transparent',
  },
  badgeText: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    letterSpacing: 1,
  },
  badgeTextActive: {
    color: '#00ff80',
  },
  badgeTextInactive: {
    color: '#6070a0',
  },
  infoText: {
    fontFamily: 'Rajdhani_500Medium',
    fontSize: 13,
    color: '#8090b8',
    lineHeight: 18,
    marginBottom: 16,
  },
  secretBox: {
    backgroundColor: 'rgba(0, 255, 255, 0.03)',
    borderWidth: 1,
    borderColor: 'rgba(0, 255, 255, 0.15)',
    padding: 12,
  },
  secretLabel: {
    fontFamily: 'Orbitron_700Bold',
    fontSize: 8,
    color: '#404070',
    letterSpacing: 1,
    marginBottom: 4,
  },
  secretText: {
    fontFamily: 'monospace',
    fontSize: 13,
    color: '#00ffff',
    letterSpacing: 0.5,
  },
});
