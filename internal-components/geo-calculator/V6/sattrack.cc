
#include "CoordTopocentric.h"
#include "CoordGeodetic.h"
#include "Observer.h"
#include "SGP4.h"
#include <time.h>

#include <iostream>

int main()
{struct tm tm, *info;
	time_t dd;
	
    libsgp4::Observer obs(51.507406923983446, -0.12773752212524414, 0.05);
    libsgp4::Tle tle = libsgp4::Tle("UK-DMC 2                ",
    	  "1 00694U 63047A   25282.43326134  .00002790  00000+0  33078-3 0  9995",
        "2 00694  30.3563 320.4866 0551104 187.9766 171.1756 14.11171726110236" );
    libsgp4::SGP4 sgp4(tle);

    std::cout << tle << std::endl;


   tm.tm_hour=0; tm.tm_min=tm.tm_sec=tm.tm_isdst=0;
  
  tm.tm_mday = 1; tm.tm_mon = 9; tm.tm_year=  2025-1900 ; dd=timegm(&tm);  // mktime dla czasu lokalnego


    //for (int i = 0; i < 10; ++i)
    {
        //libsgp4::DateTime dt = tle.Epoch().AddMinutes(100);
        libsgp4::DateTime dt = libsgp4::DateTime(2025,10,1);
        /*
         * calculate satellite position
         */
        libsgp4::Eci eci = sgp4.FindPosition(dt);
        	std::cout << dt << " " << eci.Position() << std::endl;
        		
        /*
         * get look angle for observer to satellite
         */
        libsgp4::CoordTopocentric topo = obs.GetLookAngle(eci);
        /*
         * convert satellite position to geodetic coordinates
         */
        libsgp4::CoordGeodetic geo = eci.ToGeodetic();

        std::cout << dt << " " << topo << " " << geo << std::endl;
    };

    return 0;
}

// g++ -o test sattrack.cc   -L. -lsgp4