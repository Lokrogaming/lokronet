using System;
using System.Collections.Generic;
using System.IO;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;

namespace LokroNet.Desktop;

// Modelle spiegeln die Daemon-IPC (Single Source of Truth im Go-Daemon).
// TUI (`lokronet dashboard`), CLI (`lokronet chat inbox`) und diese App
// lesen/schreiben ALLE dieselben Endpunkte – wer hier schreibt, sieht es
// sofort auch in der TUI (und umgekehrt). History liegt lokal in
// ~/.lokronet/chat-history.json (0600), Wire immer NaCl-box (E2E).

public sealed record ChatSession(
    [property: JsonPropertyName("peer_id")] string PeerId,
    [property: JsonPropertyName("code")] string Code,
    [property: JsonPropertyName("ready")] bool Ready,
    [property: JsonPropertyName("pending")] int Pending,
    [property: JsonPropertyName("updated_at")] long Updated);

public sealed record ChatMessage(
    [property: JsonPropertyName("from")] string From,
    [property: JsonPropertyName("to")] string To,
    [property: JsonPropertyName("text")] string Text,
    [property: JsonPropertyName("ts")] long Ts,
    [property: JsonPropertyName("outgoing")] bool Outgoing,
    [property: JsonPropertyName("seq")] ulong Seq);

public sealed record PresenceEntry(
    [property: JsonPropertyName("online")] bool Online,
    [property: JsonPropertyName("last_seen")] long LastSeen,
    [property: JsonPropertyName("endpoint")] string Endpoint = "",
    [property: JsonPropertyName("mode")] string Mode = "");

public sealed class IpcClient : IDisposable
{
    private readonly HttpClient _http;
    private readonly string _base;

    public IpcClient()
    {
        var dir = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), ".lokronet");
        var token = "";
        try { token = File.ReadAllText(Path.Combine(dir, "daemon.token")).Trim(); } catch { }
        var port = 37777;
        try
        {
            var raw = File.ReadAllText(Path.Combine(dir, "daemon.ipc")).Trim();
            if (int.TryParse(raw, out var p) && p is >= 1 and <= 65535) port = p;
        }
        catch { }
        _base = $"http://127.0.0.1:{port}";
        _http = new HttpClient { Timeout = TimeSpan.FromSeconds(10) };
        if (!string.IsNullOrEmpty(token))
            _http.DefaultRequestHeaders.Add("X-Lokro-Token", token);
    }

    public void Dispose() => _http.Dispose();

    private async Task<T> Get<T>(string path)
    {
        var res = await _http.GetAsync(_base + path);
        var body = await res.Content.ReadAsStringAsync();
        if (!res.IsSuccessStatusCode)
            throw new IOException($"daemon {(int)res.StatusCode}: {body}");
        return JsonSerializer.Deserialize<T>(body) ?? throw new IOException("leere Antwort");
    }

    private async Task<T> Post<T>(string path, object payload)
    {
        var json = JsonSerializer.Serialize(payload);
        var res = await _http.PostAsync(_base + path,
            new StringContent(json, Encoding.UTF8, "application/json"));
        var body = await res.Content.ReadAsStringAsync();
        if (!res.IsSuccessStatusCode)
            throw new IOException($"daemon {(int)res.StatusCode}: {body}");
        return JsonSerializer.Deserialize<T>(body) ?? throw new IOException("leere Antwort");
    }

    public Task<JsonElement> GetStatus() => Get<JsonElement>("/v1/status");
    public Task<List<ChatSession>> GetSessions() => Get<List<ChatSession>>("/v1/chat/sessions");
    public Task<List<ChatMessage>> GetInbox() => Get<List<ChatMessage>>("/v1/chat/inbox");
    public Task<Dictionary<string, PresenceEntry>> GetPresence() => Get<Dictionary<string, PresenceEntry>>("/v1/presence");

    public async Task<string> SendChat(string id, string text)
    {
        var doc = await Post<JsonElement>("/v1/chat/send", new { id, text });
        if (doc.TryGetProperty("status", out var st)) return st.GetString() ?? "ok";
        return doc.ToString();
    }

    public async Task<string> Beacon(string id)
    {
        var doc = await Post<JsonElement>("/v1/beacon", new { id });
        return doc.ToString();
    }
}
