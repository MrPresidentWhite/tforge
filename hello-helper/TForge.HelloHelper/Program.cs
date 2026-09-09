using System;
using System.Threading.Tasks;
using Windows.Security.Credentials.UI;

namespace TForge.HelloHelper;

internal static class Program
{
    // Exit codes:
    // 0 -> Verified
    // 1 -> Not available / user cancelled / failed
    static async Task<int> Main()
    {
        try
        {
            var availability = await UserConsentVerifier.CheckAvailabilityAsync();

            if (availability != UserConsentVerifierAvailability.Available)
            {
                // Kein Hello verfügbar oder gesperrt
                return 1;
            }

            var result = await UserConsentVerifier.RequestVerificationAsync("Unlock TForge agent");

            return result == UserConsentVerificationResult.Verified ? 0 : 1;
        }
        catch (Exception)
        {
            // Im Zweifel lieber sperren
            return 1;
        }
    }
}