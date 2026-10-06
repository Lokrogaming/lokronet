using System;
using System.Collections.Generic;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Threading;

namespace LokroNet.Desktop;

public partial class MainWindow : Window
{
    private readonly IpcClient _ipc = new();
    private readonly DispatcherTimer _timer = new() { Interval = TimeSpan.FromSeconds(2) };
    private string _selectedPeer = "";
    private List<ChatMessage> _inbox = new();
    private Dictionary<string, PresenceEntry> _presence = new();

    public MainWindow()
    {
        InitializeComponent();
        RefreshButton.Click += async (_, _) => await RefreshAsync();
        BeaconButton.Click += async (_, _) => await BeaconAsync();
        SendButton.Click += async (_, _) => await SendAsync();
        InputBox.KeyDown += async (_, e) => { if (e.Key == Key.Enter) await SendAsync(); };
        PeerList.SelectionChanged += (_, _) =>
        {
            if (PeerList.SelectedItem is string s)
            {
                // Eintrag: "ID  ●online  code:xxxx" -> ID ist erstes Token
                _selectedPeer = s.Split(' ')[0];
                IdBox.Text = _selectedPeer;
                RenderMessages();
            }
        };
        _timer.Tick += async (_, _) => await RefreshAsync();
        _timer.Start();
        _ = RefreshAsync();
    }

    private async Task RefreshAsync()
    {
        try
        {
            var status = await _ipc.GetStatus();
            var id = status.TryGetProperty("id", out var v) ? v.GetString() : "?";
            StatusText.Text = $"id={id}  daemon=online";
        }
        catch (Exception ex)
        {
            StatusText.Text = $"daemon offline? `lokronet daemon` starten. ({ex.Message})";
            return;
        }
        try
        {
            var sessions = await _ipc.GetSessions();
            _presence = await _ipc.GetPresence();
            _inbox = await _ipc.GetInbox();
            RenderPeers(sessions);
            RenderMessages();
            var sess = sessions.FirstOrDefault(s => s.PeerId == _selectedPeer);
            if (sess is not null)
            {
                ChatHeader.Text = $"Chat mit {_selectedPeer}";
                SessionCode.Text = $"Session-Code (out-of-band vergleichen): {sess.Code}  ready={sess.Ready}";
            }
        }
        catch { /* nächster Tick */ }
    }

    private void RenderPeers(List<ChatSession> sessions)
    {
        var items = sessions
            .OrderBy(s => s.PeerId)
            .Select(s =>
            {
                var on = _presence.TryGetValue(s.PeerId, out var p) && p.Online;
                return $"{s.PeerId}  {(on ? "●online" : "○offline")}  code:{s.Code} pending:{s.Pending}";
            })
            .ToList();
        PeerList.ItemsSource = items;
    }

    private void RenderMessages()
    {
        if (string.IsNullOrEmpty(_selectedPeer))
        {
            // Ohne Auswahl: letzte 50 aus allen
            MessageList.ItemsSource = _inbox
                .TakeLast(50)
                .Select(m => Format(m))
                .ToList();
            return;
        }
        MessageList.ItemsSource = _inbox
            .Where(m => m.From == _selectedPeer || m.To == _selectedPeer)
            .TakeLast(200)
            .Select(m => Format(m))
            .ToList();
    }

    private static string Format(ChatMessage m)
    {
        var ts = DateTimeOffset.FromUnixTimeSeconds(m.Ts).ToLocalTime().ToString("HH:mm:ss");
        var dir = m.Outgoing ? "->" : "<-";
        return $"[{ts}] {dir} {m.From}: {m.Text}";
    }

    private async Task SendAsync()
    {
        var id = (IdBox.Text ?? "").Trim();
        if (string.IsNullOrEmpty(id)) id = _selectedPeer;
        var text = (InputBox.Text ?? "").Trim();
        if (string.IsNullOrEmpty(id) || string.IsNullOrEmpty(text)) return;
        try
        {
            var st = await _ipc.SendChat(id, text);
            StatusText.Text = $"chat: {st}";
            InputBox.Text = "";
            _selectedPeer = id;
            await RefreshAsync();
        }
        catch (Exception ex)
        {
            StatusText.Text = $"senden fehlgeschlagen: {ex.Message}";
        }
    }

    private async Task BeaconAsync()
    {
        try
        {
            await _ipc.Beacon("");
            StatusText.Text = "beacon an alle gesendet";
            await RefreshAsync();
        }
        catch (Exception ex)
        {
            StatusText.Text = $"beacon fehlgeschlagen: {ex.Message}";
        }
    }
}
