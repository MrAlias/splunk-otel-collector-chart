using System.Text.Json;

var builder = WebApplication.CreateBuilder(args);
builder.Services.AddHttpClient();

var app = builder.Build();

app.MapGet("/health", () => Results.Ok(new { status = "ok" }));

app.MapPost("/chain", async (HttpContext context, IHttpClientFactory httpClientFactory) =>
{
    try
    {
        using var reader = new StreamReader(context.Request.Body);
        var body = await reader.ReadToEndAsync();

        JsonDocument doc;
        try
        {
            doc = JsonDocument.Parse(body);
        }
        catch (JsonException ex)
        {
            context.Response.ContentType = "application/json";
            context.Response.StatusCode = 400;
            await context.Response.WriteAsJsonAsync(new
            {
                service = "dotnet",
                status = 400,
                error = $"Failed to parse JSON: {ex.Message}"
            });
            return;
        }

        var targetsElement = doc.RootElement.GetProperty("targets");
        var targets = targetsElement.EnumerateArray()
            .Select(t => t.GetString())
            .Where(t => t != null)
            .ToList();

        Console.Error.WriteLine($"Received chain request with {targets.Count} targets");

        // If no targets, return success (end of chain)
        if (targets.Count == 0)
        {
            context.Response.ContentType = "application/json";
            context.Response.StatusCode = 200;
            await context.Response.WriteAsJsonAsync(new
            {
                service = "dotnet",
                status = 200,
                targets = targets,
                result = "Chain completed"
            });
            return;
        }

        // Forward to next target with remaining targets
        var nextTarget = targets[0];
        var remainingTargets = targets.Skip(1).ToList();

        Console.Error.WriteLine($"Forwarding to {nextTarget} with {remainingTargets.Count} remaining targets");

        try
        {
            var nextReq = new { targets = remainingTargets };
            var nextReqJson = JsonSerializer.Serialize(nextReq);

            var client = httpClientFactory.CreateClient();
            client.Timeout = TimeSpan.FromSeconds(10);

            var chainUrl = $"http://{nextTarget}/chain";

            // Build HttpRequestMessage to add headers
            var message = new HttpRequestMessage(HttpMethod.Post, chainUrl)
            {
                Content = new StringContent(nextReqJson, System.Text.Encoding.UTF8, "application/json")
            };

            // Forward trace headers
            string[] traceHeaders = new [] {
                "traceparent", "tracestate",
                "b3", "x-b3-traceid", "x-b3-spanid", "x-b3-sampled",
                "x-ot-span-context"
            };
            foreach (var h in traceHeaders)
            {
                if (context.Request.Headers.TryGetValue(h, out var val))
                {
                    // Some headers are restricted on HttpRequestMessage.Headers; add to content if needed
                    if (!message.Headers.TryAddWithoutValidation(h, (IEnumerable<string>)val))
                    {
                        message.Content.Headers.Remove(h);
                        message.Content.Headers.TryAddWithoutValidation(h, (IEnumerable<string>)val);
                    }
                }
            }

            var response = await client.SendAsync(message);

            var responseContent = await response.Content.ReadAsStringAsync();
            context.Response.ContentType = "application/json";
            context.Response.StatusCode = (int)response.StatusCode;
            await context.Response.WriteAsync(responseContent);
        }
        catch (Exception ex)
        {
            context.Response.ContentType = "application/json";
            context.Response.StatusCode = 502;
            await context.Response.WriteAsJsonAsync(new
            {
                service = "dotnet",
                status = 502,
                targets = targets,
                error = $"Failed to call {nextTarget}: {ex.Message}"
            });
        }
    }
    catch (Exception ex)
    {
        context.Response.ContentType = "application/json";
        context.Response.StatusCode = 400;
        await context.Response.WriteAsJsonAsync(new
        {
            service = "dotnet",
            status = 400,
            error = $"Error processing request: {ex.Message}"
        });
    }
});

var port = Environment.GetEnvironmentVariable("SERVER_PORT") ?? "8080";
app.Urls.Clear();
app.Urls.Add($"http://0.0.0.0:{port}");

Console.Error.WriteLine($"Starting server on port {port}");
await app.RunAsync();
